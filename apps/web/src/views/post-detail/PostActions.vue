<script setup lang="ts">
import {
  ChatBubbleOutlineOutlined,
  EditOutlined,
  MoreVertOutlined,
  ShareOutlined,
} from '@vicons/material';
import { computed, onScopeDispose, ref, watch } from 'vue';
import { useWebKit, XButton } from '@novelia/web-kit';

import {
  deletePost,
  lockPost,
  pinPost,
  setPostStatus,
  type Post,
  unlockPost,
  unpinPost,
} from '@/api';
import {
  DropdownMenuContent,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuTrigger,
} from 'reka-ui';
import PostFavoriteButton from './PostFavoriteButton.vue';
import PostShareDialog from './PostShareDialog.vue';
import { XActionMenuItem } from '@novelia/web-kit';
import { XConfirmDialog } from '@novelia/web-kit';
import UserBlacklistDialog from '@/components/UserBlacklistDialog.vue';
import UserModerationDialog from '@/components/UserModerationDialog.vue';
import { getApiErrorMessage, Notify } from '@novelia/web-kit';

const props = defineProps<{
  post: Post;
  canFavorite: boolean;
  favoriteLoading: boolean;
}>();

const emit = defineEmits<{
  favorite: [];
  comments: [];
  edit: [];
  deleted: [];
  updated: [patch: Partial<Pick<Post, 'pinOrder' | 'commentsLocked'>>];
  authorCommentsDeleted: [];
}>();

const { whoami } = useWebKit();
const blacklistOpen = ref(false);
const canBlockAuthor = computed(
  () => whoami.value.isSignedIn && !isOwner.value,
);
const actionLoading = ref(false);
const copying = ref(false);
const shareOpen = ref(false);
const confirmationAction = ref<'delete' | 'hide'>();
const userModerationAction = ref<'strike' | 'ban'>();
let context = 0;
watch(
  () => [props.post.id, whoami.value.user?.id],
  (next, previous) => {
    if (next.some((value, index) => value !== previous[index])) {
      context++;
      actionLoading.value = false;
      shareOpen.value = false;
      blacklistOpen.value = false;
      confirmationAction.value = undefined;
      userModerationAction.value = undefined;
    }
  },
  { flush: 'sync' },
);
onScopeDispose(() => context++);
const asAdmin = computed(() => whoami.value.asAdmin);
const isOwner = computed(() => whoami.value.user?.id === props.post.authorId);
const canManagePost = computed(() => isOwner.value || asAdmin.value);
const canDelete = computed(
  () =>
    asAdmin.value ||
    (isOwner.value &&
      Date.now() <= new Date(props.post.createdAt).getTime() + 20 * 60_000),
);
const canModerateAuthor = computed(() => asAdmin.value && !isOwner.value);
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

async function copyPost() {
  if (copying.value) return;
  copying.value = true;
  try {
    await navigator.clipboard.writeText(props.post.content);
    Notify.success('帖子原文已复制');
  } catch {
    Notify.error('复制失败，请手动选择帖子内容复制');
  } finally {
    copying.value = false;
  }
}

function editPost() {
  emit('edit');
}

function removePost() {
  void updateModeration(
    () => deletePost(props.post.id),
    { status: 1 },
    '帖子已删除',
    '删除帖子失败',
  );
}

async function updateModeration(
  request: () => Promise<unknown>,
  patch: Partial<Pick<Post, 'pinOrder' | 'commentsLocked' | 'status'>>,
  successMessage: string,
  failureMessage: string,
) {
  if (actionLoading.value || !canManagePost.value) return;
  const requestContext = context;
  const isCurrent = () => requestContext === context;
  actionLoading.value = true;
  try {
    await request();
    if (!isCurrent()) return;
    Notify.success(successMessage);
    if (patch.status != null && patch.status !== 0) emit('deleted');
    else emit('updated', patch);
  } catch (reason) {
    if (!isCurrent()) return;
    const message = await getApiErrorMessage(reason, failureMessage);
    if (isCurrent()) Notify.error(message);
  } finally {
    if (isCurrent()) actionLoading.value = false;
  }
}

function hidePost() {
  void updateModeration(
    () => setPostStatus(props.post.id, 1),
    { status: 1 },
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
  const request = () =>
    pinOrder == null
      ? unpinPost(props.post.id)
      : pinPost(props.post.id, pinOrder);
  void updateModeration(
    request,
    { pinOrder },
    pinOrder == null ? '已取消置顶' : '帖子已置顶',
    pinOrder == null ? '取消置顶失败' : '置顶帖子失败',
  );
}

function toggleLock() {
  const commentsLocked = !props.post.commentsLocked;
  const request = () =>
    commentsLocked ? lockPost(props.post.id) : unlockPost(props.post.id);
  void updateModeration(
    request,
    { commentsLocked },
    commentsLocked ? '评论区已锁定' : '评论区已开放',
    commentsLocked ? '锁定评论失败' : '开放评论失败',
  );
}
</script>

<template>
  <section class="relative" aria-label="帖子快捷操作">
    <div class="flex flex-wrap items-center gap-2">
      <PostFavoriteButton
        v-if="canFavorite"
        prominent
        :favorited="post.favorited"
        :loading="favoriteLoading"
        @toggle="emit('favorite')"
      />
      <XButton
        as="a"
        href="#comments"
        variant="outline"
        size="sm"
        class="text-ink!"
        aria-controls="comments"
        @click.prevent="emit('comments')"
      >
        <ChatBubbleOutlineOutlined class="size-4" aria-hidden="true" />
        评论
      </XButton>
      <XButton
        v-if="canManagePost"
        variant="outline"
        size="sm"
        class="text-ink!"
        :disabled="actionLoading"
        @click="editPost"
      >
        <EditOutlined class="size-4" aria-hidden="true" />
        编辑
      </XButton>
      <XButton
        variant="ghost"
        size="none"
        class="min-h-9 px-1.5 text-ink!"
        aria-label="分享帖子"
        title="分享帖子"
        aria-haspopup="dialog"
        :aria-expanded="shareOpen"
        @click="shareOpen = true"
      >
        <ShareOutlined class="size-5" aria-hidden="true" />
      </XButton>
      <DropdownMenuRoot>
        <DropdownMenuTrigger as-child>
          <XButton
            variant="ghost"
            size="none"
            class="min-h-9 px-1.5 text-ink!"
            aria-label="更多操作"
            title="更多操作"
            :disabled="actionLoading"
          >
            <MoreVertOutlined class="size-5" aria-hidden="true" />
          </XButton>
        </DropdownMenuTrigger>
        <DropdownMenuPortal>
          <DropdownMenuContent
            side="bottom"
            align="end"
            :side-offset="6"
            :collision-padding="8"
            class="z-30 w-40 rounded-md border border-border bg-surface p-1 shadow-xl outline-none"
          >
            <XActionMenuItem :disabled="copying" @activate="copyPost">
              复制原文
            </XActionMenuItem>
            <template v-if="asAdmin">
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
            <XActionMenuItem
              v-if="canBlockAuthor"
              :disabled="actionLoading"
              @activate="blacklistOpen = true"
            >
              {{ post.authorBlocked ? '取消拉黑' : '拉黑作者' }}
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
          </DropdownMenuContent>
        </DropdownMenuPortal>
      </DropdownMenuRoot>
    </div>
    <PostShareDialog
      :open="shareOpen"
      :post="post"
      @update:open="shareOpen = $event"
    />
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
    <UserBlacklistDialog
      v-if="blacklistOpen && canBlockAuthor"
      :user-id="post.authorId"
      :username="post.authorUsername"
      :author-blocked="post.authorBlocked"
      @close="blacklistOpen = false"
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
