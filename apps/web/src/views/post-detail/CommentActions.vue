<script setup lang="ts">
import { MoreVertOutlined } from '@vicons/material';
import {
  DropdownMenuContent,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuTrigger,
} from 'reka-ui';
import { computed, onBeforeUnmount, ref } from 'vue';

import type { PostComment } from '@/api';
import { useWebKit, XButton } from '@novelia/web-kit';
import { XActionMenuItem } from '@novelia/web-kit';
import { XConfirmDialog } from '@novelia/web-kit';
import UserBlacklistDialog from '@/components/UserBlacklistDialog.vue';
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

const { whoami } = useWebKit();
const blacklistOpen = ref(false);
const canBlockAuthor = computed(
  () => whoami.value.isSignedIn && !isOwner.value,
);
const commentStore = useCommentStore();
const submitting = ref(false);
const copying = ref(false);
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

const asAdmin = computed(() => whoami.value.asAdmin);
const isOwner = computed(
  () => whoami.value.user?.id === props.comment.authorId,
);
const isPublished = computed(() => props.comment.status === 0);
const canReply = computed(
  () => isPublished.value && whoami.value.isSignedIn && !props.locked,
);
const canEdit = computed(
  () =>
    isPublished.value &&
    (asAdmin.value || (isOwner.value && now.value <= modificationDeadline)),
);
const canDelete = computed(() => asAdmin.value || canEdit.value);
const canModerateAuthor = computed(() => asAdmin.value && !isOwner.value);
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

async function copyComment() {
  if (copying.value) return;
  copying.value = true;
  try {
    await navigator.clipboard.writeText(props.comment.content);
    Notify.success('评论原文已复制');
  } catch {
    Notify.error('复制失败，请手动选择评论内容复制');
  } finally {
    copying.value = false;
  }
}

async function removeComment() {
  submitting.value = true;
  try {
    await commentStore.deleteComment(props.comment.id, asAdmin.value);
    Notify.success('评论已删除');
    emit('statusChanged', 2);
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '删除评论失败'));
  } finally {
    submitting.value = false;
  }
}

async function unhideComment() {
  if (submitting.value || !asAdmin.value || props.comment.status !== 1) return;
  submitting.value = true;
  try {
    await commentStore.unhideComment(props.comment.id);
    Notify.success('评论已解除隐藏');
    emit('statusChanged', 0);
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '解除隐藏失败'));
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
  <div class="ml-auto flex items-center gap-1">
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
    <DropdownMenuRoot>
      <DropdownMenuTrigger as-child>
        <XButton
          variant="ghost"
          size="icon-xs"
          aria-label="更多操作"
          title="更多操作"
        >
          <MoreVertOutlined class="size-4" aria-hidden="true" />
        </XButton>
      </DropdownMenuTrigger>
      <DropdownMenuPortal>
        <DropdownMenuContent
          side="bottom"
          align="end"
          :side-offset="6"
          :collision-padding="8"
          class="z-30 w-32 rounded-md border border-border bg-surface p-1 shadow-xl outline-none"
        >
          <XActionMenuItem :disabled="copying" @activate="copyComment">
            复制原文
          </XActionMenuItem>
          <XActionMenuItem
            v-if="asAdmin && isPublished"
            :disabled="submitting"
            @activate="confirmationAction = 'hide'"
          >
            隐藏评论
          </XActionMenuItem>
          <XActionMenuItem
            v-if="asAdmin && comment.status === 1"
            :disabled="submitting"
            @activate="unhideComment"
          >
            解除隐藏
          </XActionMenuItem>
          <XActionMenuItem
            v-if="canDelete"
            danger
            :disabled="submitting"
            @activate="confirmationAction = 'delete'"
          >
            删除评论
          </XActionMenuItem>
          <XActionMenuItem
            v-if="canBlockAuthor"
            :disabled="submitting"
            @activate="blacklistOpen = true"
          >
            {{ comment.authorBlocked ? '取消拉黑' : '拉黑作者' }}
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
        </DropdownMenuContent>
      </DropdownMenuPortal>
    </DropdownMenuRoot>

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
    <UserBlacklistDialog
      v-if="blacklistOpen && canBlockAuthor"
      :user-id="comment.authorId"
      :username="comment.authorUsername"
      :author-blocked="comment.authorBlocked"
      @close="blacklistOpen = false"
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
