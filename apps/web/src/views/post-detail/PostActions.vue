<script setup lang="ts">
import { StarBorderOutlined, StarFilled } from '@vicons/material';
import { computed, ref, watch } from 'vue';

import {
  authUser,
  deletePost,
  lockPost,
  pinPost,
  setPostFavorite,
  setPostStatus,
  type Post,
  unlockPost,
  unpinPost,
} from '@/api';
import { XButton } from '@novelia/web-kit';
import { XActionMenu } from '@novelia/web-kit';
import { XActionMenuItem } from '@novelia/web-kit';
import { XConfirmDialog } from '@novelia/web-kit';
import UserModerationDialog from '@/components/UserModerationDialog.vue';
import { getApiErrorMessage, Notify } from '@novelia/web-kit';

const props = defineProps<{ post: Post }>();

const emit = defineEmits<{
  edit: [];
  deleted: [];
  updated: [post: Post];
  authorCommentsDeleted: [];
}>();

const favorited = ref(props.post.favorited);
const favoriteLoading = ref(false);
const actionLoading = ref(false);
const confirmationAction = ref<'delete' | 'hide'>();
const userModerationAction = ref<'strike' | 'ban'>();
const isAdmin = computed(() => authUser.value?.role === 'admin');
const isOwner = computed(() => authUser.value?.id === props.post.authorId);
const canManagePost = computed(() => isOwner.value || isAdmin.value);
const canDelete = computed(
  () =>
    isAdmin.value ||
    (isOwner.value &&
      Date.now() <= new Date(props.post.createdAt).getTime() + 20 * 60_000),
);
const canModerateAuthor = computed(() => isAdmin.value && !isOwner.value);
const moderationEvidence = computed(() =>
  [
    `论坛帖子 #${props.post.id}：${props.post.title}`,
    new URL(`/p/${props.post.id}`, window.location.origin).toString(),
  ].join('\n'),
);
const confirmation = computed(() =>
  confirmationAction.value === 'delete'
    ? {
        title: '删除帖子',
        description: '确定删除这篇帖子吗？',
        confirmLabel: '删除帖子',
      }
    : {
        title: '隐藏帖子',
        description: '确定隐藏这篇帖子吗？',
        confirmLabel: '隐藏帖子',
      },
);

async function toggleFavorite() {
  if (!authUser.value || favoriteLoading.value) return;
  favoriteLoading.value = true;
  const nextValue = !favorited.value;
  try {
    await setPostFavorite(props.post.id, nextValue);
    favorited.value = nextValue;
    emit('updated', { ...props.post, favorited: nextValue });
    Notify.success(nextValue ? '帖子已收藏' : '已取消收藏');
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '更新收藏失败'));
  } finally {
    favoriteLoading.value = false;
  }
}

function editPost() {
  emit('edit');
}

async function removePost() {
  actionLoading.value = true;
  try {
    await deletePost(props.post.id);
    Notify.success('帖子已删除');
    emit('deleted');
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '删除帖子失败'));
  } finally {
    actionLoading.value = false;
  }
}

async function updateModeration(
  request: Promise<unknown>,
  nextPost: Post,
  successMessage: string,
  failureMessage: string,
) {
  actionLoading.value = true;
  try {
    await request;
    Notify.success(successMessage);
    if (nextPost.status !== 0) emit('deleted');
    else emit('updated', nextPost);
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, failureMessage));
  } finally {
    actionLoading.value = false;
  }
}

function hidePost() {
  void updateModeration(
    setPostStatus(props.post.id, 1),
    {
      ...props.post,
      status: 1,
    },
    '帖子已隐藏',
    '隐藏帖子失败',
  );
}

function confirmAction() {
  const action = confirmationAction.value;
  confirmationAction.value = undefined;
  if (action === 'delete') void removePost();
  else if (action === 'hide') hidePost();
}

function handleConfirmationOpenChange(open: boolean) {
  if (!open) confirmationAction.value = undefined;
}

function handleUserModerationOpenChange(open: boolean) {
  if (!open) userModerationAction.value = undefined;
}

function togglePin() {
  const pinOrder = props.post.pinOrder == null ? 0 : null;
  const request =
    pinOrder == null
      ? unpinPost(props.post.id)
      : pinPost(props.post.id, pinOrder);
  void updateModeration(
    request,
    { ...props.post, pinOrder },
    pinOrder == null ? '已取消置顶' : '帖子已置顶',
    pinOrder == null ? '取消置顶失败' : '置顶帖子失败',
  );
}

function toggleLock() {
  const commentsLocked = !props.post.commentsLocked;
  const request = commentsLocked
    ? lockPost(props.post.id)
    : unlockPost(props.post.id);
  void updateModeration(
    request,
    { ...props.post, commentsLocked },
    commentsLocked ? '评论区已锁定' : '评论区已开放',
    commentsLocked ? '锁定评论失败' : '开放评论失败',
  );
}

watch(
  () => props.post.favorited,
  (value) => {
    favorited.value = value;
  },
);
</script>

<template>
  <section class="relative border-t border-divider py-3" aria-label="帖子操作">
    <div class="flex flex-wrap items-center gap-2">
      <XButton
        v-if="authUser"
        :variant="favorited ? 'ghost-active' : 'ghost'"
        size="sm"
        :disabled="favoriteLoading"
        :aria-pressed="favorited"
        @click="toggleFavorite"
      >
        <StarFilled v-if="favorited" class="size-4" aria-hidden="true" />
        <StarBorderOutlined v-else class="size-4" aria-hidden="true" />
        {{ favoriteLoading ? '处理中…' : favorited ? '取消收藏' : '收藏' }}
      </XButton>

      <div v-if="canManagePost">
        <XActionMenu>
          <XActionMenuItem :disabled="actionLoading" @activate="editPost">
            编辑帖子
          </XActionMenuItem>
          <template v-if="isAdmin">
            <XActionMenuItem :disabled="actionLoading" @activate="togglePin">
              {{ post.pinOrder == null ? '置顶帖子' : '取消置顶' }}
            </XActionMenuItem>
            <XActionMenuItem :disabled="actionLoading" @activate="toggleLock">
              {{ post.commentsLocked ? '开放评论' : '锁定评论' }}
            </XActionMenuItem>
            <XActionMenuItem
              :disabled="actionLoading"
              @activate="confirmationAction = 'hide'"
            >
              隐藏帖子
            </XActionMenuItem>
          </template>
          <XActionMenuItem
            v-if="canDelete"
            danger
            :disabled="actionLoading"
            @activate="confirmationAction = 'delete'"
          >
            删除帖子
          </XActionMenuItem>
          <template v-if="canModerateAuthor">
            <XActionMenuItem
              danger
              :disabled="actionLoading"
              @activate="userModerationAction = 'strike'"
            >
              处罚作者
            </XActionMenuItem>
            <XActionMenuItem
              danger
              :disabled="actionLoading"
              @activate="userModerationAction = 'ban'"
            >
              封禁作者
            </XActionMenuItem>
          </template>
        </XActionMenu>
      </div>
    </div>
    <XConfirmDialog
      :open="confirmationAction != null"
      :title="confirmation.title"
      :description="confirmation.description"
      :confirm-label="confirmation.confirmLabel"
      :loading="actionLoading"
      danger
      @update:open="handleConfirmationOpenChange"
      @confirm="confirmAction"
    />
    <UserModerationDialog
      v-if="userModerationAction"
      open
      :action="userModerationAction"
      :user-id="post.authorId"
      :username="post.authorUsername"
      :evidence="moderationEvidence"
      @update:open="handleUserModerationOpenChange"
      @comments-deleted="emit('authorCommentsDeleted')"
    />
  </section>
</template>
