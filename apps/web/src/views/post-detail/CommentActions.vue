<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';

import { authUser, type PostComment } from '@/api';
import { XButton } from '@novelia/web-kit';
import { XActionMenu } from '@novelia/web-kit';
import { XActionMenuItem } from '@novelia/web-kit';
import { XConfirmDialog } from '@novelia/web-kit';
import UserModerationDialog from '@/components/UserModerationDialog.vue';
import { getApiErrorMessage, Notify } from '@novelia/web-kit';
import { useCommentStore } from '@/stores/comment';

const props = defineProps<{
  comment: PostComment;
  locked: boolean;
  postId: number;
  replying: boolean;
}>();

const emit = defineEmits<{
  reply: [];
  edit: [];
  statusChanged: [status: number];
  authorCommentsDeleted: [];
}>();

const commentStore = useCommentStore();
const submitting = ref(false);
const now = ref(Date.now());
const confirmationAction = ref<'delete' | 'hide'>();
const userModerationAction = ref<'strike' | 'ban'>();

const modificationDeadline =
  new Date(props.comment.createdAt).getTime() + 20 * 60_000;
const expiryTimer = window.setTimeout(
  () => {
    now.value = Date.now();
  },
  Math.max(0, modificationDeadline - Date.now() + 50),
);

const isAdmin = computed(() => authUser.value?.role === 'admin');
const isOwner = computed(() => authUser.value?.id === props.comment.authorId);
const canReply = computed(() => Boolean(authUser.value) && !props.locked);
const canEdit = computed(
  () => isAdmin.value || (isOwner.value && now.value <= modificationDeadline),
);
const canModerateAuthor = computed(() => isAdmin.value && !isOwner.value);
const hasMenu = computed(() => canEdit.value || isAdmin.value);
const hasActions = computed(() => canReply.value || hasMenu.value);
const moderationEvidence = computed(() =>
  [
    `论坛评论 #${props.comment.id}（帖子 #${props.postId}）`,
    new URL(
      `/p/${props.postId}#comment-${props.comment.id}`,
      window.location.origin,
    ).toString(),
  ].join('\n'),
);
const confirmation = computed(() =>
  confirmationAction.value === 'delete'
    ? {
        title: '删除评论',
        description: '确定删除这条评论吗？',
        confirmLabel: '删除评论',
      }
    : {
        title: '隐藏评论',
        description: '确定隐藏这条评论吗？',
        confirmLabel: '隐藏评论',
      },
);

onBeforeUnmount(() => window.clearTimeout(expiryTimer));

async function removeComment() {
  submitting.value = true;
  try {
    await commentStore.deleteComment(props.comment.id, isAdmin.value);
    Notify.success('评论已删除');
    emit('statusChanged', 2);
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '删除评论失败'));
  } finally {
    submitting.value = false;
  }
}

async function hideComment() {
  submitting.value = true;
  try {
    await commentStore.hideComment(props.comment.id);
    Notify.success('评论已隐藏');
    emit('statusChanged', 1);
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '隐藏评论失败'));
  } finally {
    submitting.value = false;
  }
}

function confirmAction() {
  const action = confirmationAction.value;
  confirmationAction.value = undefined;
  if (action === 'delete') void removeComment();
  else if (action === 'hide') void hideComment();
}

function handleConfirmationOpenChange(open: boolean) {
  if (!open) confirmationAction.value = undefined;
}

function handleUserModerationOpenChange(open: boolean) {
  if (!open) userModerationAction.value = undefined;
}
</script>

<template>
  <div v-if="hasActions" class="ml-auto flex items-center gap-1">
    <XButton
      v-if="canReply"
      variant="ghost"
      size="xs"
      :aria-expanded="replying"
      @click="emit('reply')"
    >
      回复
    </XButton>
    <XButton v-if="canEdit" variant="ghost" size="xs" @click="emit('edit')">
      编辑
    </XButton>
    <XActionMenu v-if="hasMenu" compact side="bottom" align="end">
      <XActionMenuItem
        v-if="isAdmin"
        :disabled="submitting"
        @activate="confirmationAction = 'hide'"
      >
        隐藏评论
      </XActionMenuItem>
      <XActionMenuItem
        danger
        :disabled="submitting"
        @activate="confirmationAction = 'delete'"
      >
        删除评论
      </XActionMenuItem>
      <template v-if="canModerateAuthor">
        <XActionMenuItem
          danger
          :disabled="submitting"
          @activate="userModerationAction = 'strike'"
        >
          处罚作者
        </XActionMenuItem>
        <XActionMenuItem
          danger
          :disabled="submitting"
          @activate="userModerationAction = 'ban'"
        >
          封禁作者
        </XActionMenuItem>
      </template>
    </XActionMenu>

    <XConfirmDialog
      :open="confirmationAction != null"
      :title="confirmation.title"
      :description="confirmation.description"
      :confirm-label="confirmation.confirmLabel"
      :loading="submitting"
      danger
      @update:open="handleConfirmationOpenChange"
      @confirm="confirmAction"
    />
    <UserModerationDialog
      v-if="userModerationAction"
      open
      :action="userModerationAction"
      :user-id="comment.authorId"
      :username="comment.authorUsername"
      :evidence="moderationEvidence"
      @update:open="handleUserModerationOpenChange"
      @comments-deleted="emit('authorCommentsDeleted')"
    />
  </div>
</template>
