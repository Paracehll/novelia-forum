<script setup lang="ts">
import { computed, onScopeDispose, ref, watch } from 'vue';
import {
  getApiErrorMessage,
  Notify,
  useWebKit,
  XConfirmDialog,
} from '@novelia/web-kit';
import { setUserBlocked } from '@/api';
import { usePostStore } from '@/stores/post';
import { useCommentStore } from '@/stores/comment';

const props = defineProps<{
  userId: number;
  username: string;
  authorBlocked: boolean;
}>();
const emit = defineEmits<{ close: [] }>();
const { whoami } = useWebKit();
const posts = usePostStore();
const comments = useCommentStore();
const pending = ref(false);
let context = 0;
let active = true;
onScopeDispose(() => {
  active = false;
});
// Keep the requested operation stable while the content cache updates.
const blocked = props.authorBlocked;
const title = computed(
  () => `${blocked ? '取消拉黑' : '拉黑'} @${props.username}`,
);
watch(
  () => [props.userId, whoami.value.user?.id],
  () => {
    context++;
    emit('close');
  },
  { flush: 'sync' },
);

async function confirm() {
  const viewerId = whoami.value.user?.id;
  const userId = props.userId;
  if (pending.value || !viewerId || viewerId === userId) return;
  const requestContext = context;
  const isCurrent = () =>
    requestContext === context && whoami.value.user?.id === viewerId;
  pending.value = true;
  try {
    await setUserBlocked(userId, !blocked);
    if (!isCurrent()) return;
    // Keep content caches in sync even if navigation closed this dialog.
    posts.refreshBlacklist(userId, !blocked);
    comments.refreshBlacklist(userId, !blocked);
    if (active) {
      Notify.success(blocked ? '已取消拉黑' : '已拉黑作者');
      emit('close');
    }
  } catch (reason) {
    if (!active || !isCurrent()) return;
    const message = await getApiErrorMessage(
      reason,
      blocked ? '取消拉黑失败' : '拉黑失败',
    );
    if (active && isCurrent()) Notify.error(message);
  } finally {
    pending.value = false;
  }
}
</script>

<template>
  <XConfirmDialog
    open
    :title="title"
    :description="
      blocked
        ? '取消后，该作者的帖子和评论将重新出现在列表中。'
        : '拉黑后，帖子和收藏列表将过滤该作者的帖子，评论区将过滤其评论及其发起的评论串。仅对你生效。'
    "
    :confirm-label="blocked ? '取消拉黑' : '拉黑作者'"
    :loading="pending"
    :danger="!blocked"
    @update:open="
      (open) => {
        if (!open && !pending) emit('close');
      }
    "
    @confirm="confirm"
  />
</template>
