<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue';

import { authUser, type PostComment } from '@/api';
import { XButton } from '@novelia/web-kit';
import MarkdownContent from '@/components/markdown/MarkdownContent.vue';
import MarkdownEditor from '@/components/markdown/MarkdownEditor.vue';
import MarkdownHelpDialog from '@/components/markdown/MarkdownHelpDialog.vue';
import { useCommentValidation } from '@/composables/useCommentValidation';
import { useUnsavedChangesGuard } from '@/composables/useUnsavedChangesGuard';
import { getApiErrorMessage, Notify } from '@novelia/web-kit';
import { useCommentStore } from '@/stores/comment';

import CommentActions from './CommentActions.vue';
import CommentComposer from './CommentComposer.vue';

const props = defineProps<{
  comment: PostComment;
  locked: boolean;
  postId: number;
  replying: boolean;
}>();

const emit = defineEmits<{
  reply: [comment: PostComment];
  cancelReply: [];
  created: [comment: PostComment];
  statusChanged: [id: number, status: number];
  authorCommentsDeleted: [];
}>();

const commentStore = useCommentStore();
const editing = ref(false);
const content = ref(props.comment.content);
const submitting = ref(false);
const commentHintId = useId();
const {
  length: commentLength,
  hint: commentHint,
  canSubmit: canSave,
} = useCommentValidation(content, submitting);
const showModeratedContent = ref(false);

const isPublished = computed(() => props.comment.status === 0);
const isAdmin = computed(() => authUser.value?.role === 'admin');
const statusLabel = computed(() =>
  props.comment.status === 2 ? '该评论已删除' : '该评论已隐藏',
);
watch(
  [() => props.comment.id, () => props.comment.status, () => authUser.value],
  () => {
    showModeratedContent.value = false;
  },
);
const hasUnsavedChanges = computed(
  () => editing.value && content.value !== props.comment.content,
);

useUnsavedChangesGuard(hasUnsavedChanges);

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function startEditing() {
  if (props.replying) emit('cancelReply');
  content.value = props.comment.content;
  editing.value = true;
}

async function saveEdit() {
  const value = content.value.trim();
  if (!canSave.value) return;
  submitting.value = true;
  try {
    await commentStore.updateComment(props.comment.id, value);
    editing.value = false;
    Notify.success('评论修改已保存');
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '更新评论失败'));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <article
    :id="`comment-${comment.id}`"
    class="py-4"
    :class="comment.rootId != null ? 'ml-6 pl-4 sm:ml-10 sm:pl-5' : ''"
  >
    <header class="flex items-center gap-2 text-xs text-muted">
      <span class="font-medium text-ink">{{ comment.authorUsername }}</span>
      <span aria-hidden="true">·</span>
      <time :datetime="comment.createdAt">
        {{ formatDate(comment.createdAt) }}
      </time>
      <CommentActions
        v-if="isPublished && !editing"
        :comment="comment"
        :locked="locked"
        :post-id="postId"
        :replying="replying"
        @reply="emit('reply', comment)"
        @edit="startEditing"
        @status-changed="emit('statusChanged', comment.id, $event)"
        @author-comments-deleted="emit('authorCommentsDeleted')"
      />
    </header>
    <div v-if="!isPublished" class="mt-2">
      <XButton
        v-if="isAdmin"
        variant="plain"
        size="none"
        class="text-sm text-muted hover:text-primary"
        :aria-expanded="showModeratedContent"
        :aria-controls="`comment-${comment.id}-moderated-content`"
        @click="showModeratedContent = !showModeratedContent"
      >
        {{ statusLabel }}
      </XButton>
      <p v-else class="text-sm text-muted">{{ statusLabel }}</p>
      <div v-if="isAdmin" :id="`comment-${comment.id}-moderated-content`">
        <MarkdownContent
          v-if="showModeratedContent"
          class="mt-2"
          mode="comment"
          :source="comment.content"
        />
      </div>
    </div>
    <form v-else-if="editing" class="mt-2" @submit.prevent="saveEdit">
      <MarkdownEditor
        v-model="content"
        mode="comment"
        :rows="5"
        :disabled="submitting"
        :described-by="commentHint ? commentHintId : undefined"
        :invalid="commentLength > 1000"
      />
      <div class="mt-2 flex items-center justify-between gap-3 text-xs">
        <p
          v-if="commentHint"
          :id="commentHintId"
          :class="content ? 'text-orange-700' : 'text-muted'"
          aria-live="polite"
        >
          {{ commentHint }}
        </p>
        <p class="ml-auto text-muted">{{ commentLength }} / 1000</p>
      </div>
      <div class="mt-3 flex items-center justify-between gap-3">
        <MarkdownHelpDialog mode="comment" />
        <div class="flex gap-2">
          <XButton
            variant="ghost"
            size="xs"
            :disabled="submitting"
            @click="editing = false"
          >
            取消
          </XButton>
          <XButton type="submit" size="xs" :disabled="!canSave">
            {{ submitting ? '保存中…' : '保存' }}
          </XButton>
        </div>
      </div>
    </form>
    <MarkdownContent
      v-else
      class="mt-2"
      mode="comment"
      :source="comment.content"
    />

    <CommentComposer
      v-if="replying"
      :id="`comment-reply-composer-${comment.id}`"
      :post-id="postId"
      :locked="locked"
      :reply-to="comment"
      @created="emit('created', $event)"
      @cancel-reply="emit('cancelReply')"
    />
  </article>
</template>
