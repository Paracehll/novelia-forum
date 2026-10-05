<script setup lang="ts">
import { computed, nextTick, ref } from 'vue';

import type { PostComment } from '@/api';
import {
  COMMENT_REPLY_PAGE_SIZE,
  useCommentReplyPageQuery,
} from '@/stores/comment';
import { XButton } from '@novelia/web-kit';
import { XPagination } from '@novelia/web-kit';

import CommentListItem from './CommentListItem.vue';

const REPLY_PAGE_SIZE = COMMENT_REPLY_PAGE_SIZE;

const props = defineProps<{
  comment: PostComment;
  locked: boolean;
  postId: number;
  replyToId?: number;
}>();

const emit = defineEmits<{
  reply: [comment: PostComment];
  cancelReply: [];
  created: [comment: PostComment];
  statusChanged: [id: number, status: number];
  authorCommentsDeleted: [];
}>();

const replyPage = ref(1);
const { replies, total, loading, error, refresh, retry } =
  useCommentReplyPageQuery(
    () => props.postId,
    () => props.comment.id,
    replyPage,
    REPLY_PAGE_SIZE,
    () => props.comment.replyCount > 0,
  );
const totalPages = computed(() =>
  Math.max(1, Math.ceil(total.value / REPLY_PAGE_SIZE)),
);

async function handleCreated(comment: PostComment) {
  emit('created', comment);
  if (comment.rootId !== props.comment.id) return;
  replyPage.value = 1;
  await nextTick();
  await refresh();
}
</script>

<template>
  <section>
    <CommentListItem
      :comment="comment"
      :locked="locked"
      :post-id="postId"
      :replying="replyToId === comment.id"
      @reply="emit('reply', $event)"
      @cancel-reply="emit('cancelReply')"
      @created="handleCreated"
      @status-changed="(id, status) => emit('statusChanged', id, status)"
      @author-comments-deleted="emit('authorCommentsDeleted')"
    />

    <div v-if="comment.replyCount > 0" class="border-t border-divider/60">
      <p v-if="loading" class="py-4 pl-10 text-sm text-muted">正在加载回复…</p>
      <div v-else-if="error" class="flex items-center gap-3 py-4 pl-10 text-sm">
        <span class="text-red-600">{{ error }}</span>
        <XButton variant="outline" size="xs" @click="retry">重试</XButton>
      </div>
      <template v-else>
        <CommentListItem
          v-for="reply in replies"
          :key="reply.id"
          :comment="reply"
          :locked="locked"
          :post-id="postId"
          :replying="replyToId === reply.id"
          @reply="emit('reply', $event)"
          @cancel-reply="emit('cancelReply')"
          @created="handleCreated"
          @status-changed="(id, status) => emit('statusChanged', id, status)"
          @author-comments-deleted="emit('authorCommentsDeleted')"
        />
        <XPagination
          v-if="totalPages > 1"
          :page="replyPage"
          :total-pages="totalPages"
          @change="replyPage = $event"
        />
      </template>
    </div>
  </section>
</template>
