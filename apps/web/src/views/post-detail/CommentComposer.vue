<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue';

import type { PostComment } from '@/api';
import CommentRulesNotice from './CommentRulesNotice.vue';
import MarkdownEditor from '@/components/markdown/MarkdownEditor.vue';
import MarkdownHelpDialog from '@/components/markdown/MarkdownHelpDialog.vue';
import { useCommentValidation } from '@/composables/useCommentValidation';
import { getApiErrorMessage, Notify } from '@novelia/web-kit';
import { useCommentStore } from '@/stores/comment';
import { useDraftStore } from '@/stores/draft';
import { useWebKit, XButton } from '@novelia/web-kit';

const props = defineProps<{
  postId: number;
  locked: boolean;
  replyTo?: PostComment;
}>();

const emit = defineEmits<{
  created: [comment: PostComment];
  cancelReply: [];
}>();

const { whoami } = useWebKit();
const commentStore = useCommentStore();
const draftStore = useDraftStore();
const content = ref('');
const submitting = ref(false);
const commentHintId = useId();
const commentPlaceholders = [
  '允许别人喜欢你不喜欢的，也允许别人不喜欢你喜欢的',
  '就事论事，请勿人身攻击',
];
const commentPlaceholder =
  commentPlaceholders[Math.floor(Math.random() * commentPlaceholders.length)];
const {
  length: commentLength,
  hint: commentHint,
  canSubmit,
} = useCommentValidation(content, submitting);
const draftKey = computed(() => {
  const user = whoami.value.user;
  return user
    ? `${props.postId}:${user.id}:${props.replyTo ? `reply:${props.replyTo.id}` : 'root'}`
    : '';
});

watch(
  draftKey,
  (key) => {
    const draft = key ? draftStore.getCommentDraft(key) : '';
    const replyTo = props.replyTo;
    const mention =
      replyTo?.rootId != null && replyTo.authorId !== whoami.value.user?.id
        ? `@${replyTo.authorUsername} `
        : '';
    content.value = draft || mention;
  },
  { immediate: true },
);

watch(content, (value) => {
  if (draftKey.value) draftStore.saveCommentDraft(draftKey.value, value);
});

async function submitComment() {
  const value = content.value.trim();
  if (!canSubmit.value || props.locked) return;
  submitting.value = true;
  try {
    const comment = await commentStore.createComment(props.postId, {
      content: value,
      rootId: props.replyTo?.rootId ?? props.replyTo?.id,
    });
    content.value = '';
    draftStore.clearCommentDraft(draftKey.value);
    Notify.success('评论已发表');
    emit('created', comment);
  } catch (error) {
    Notify.error(await getApiErrorMessage(error, '评论发布失败'));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <section :class="replyTo ? 'mt-3' : 'border-b border-divider py-4'">
    <div
      v-if="locked"
      :class="['py-2 text-sm text-orange-700', { 'mt-4': replyTo }]"
    >
      评论区已锁定，暂时无法发表新评论。
    </div>

    <div
      v-else-if="!whoami.isSignedIn"
      :class="['py-2 text-sm text-muted', { 'mt-4': replyTo }]"
    >
      登录后即可参与评论，请使用页面右上角的登录入口。
    </div>

    <form v-else @submit.prevent="submitComment">
      <CommentRulesNotice class="mb-3" />
      <MarkdownEditor
        v-model="content"
        mode="comment"
        :rows="3"
        :placeholder="
          replyTo ? `回复 @${replyTo.authorUsername}…` : commentPlaceholder
        "
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
      <div class="mt-3 flex flex-wrap items-center justify-between gap-3">
        <MarkdownHelpDialog mode="comment" />
        <div class="flex items-center gap-2">
          <XButton
            v-if="replyTo"
            variant="outline"
            :disabled="submitting"
            @click="emit('cancelReply')"
          >
            取消
          </XButton>
          <XButton type="submit" :disabled="!canSubmit">
            {{ submitting ? '发表中…' : '发表' }}
          </XButton>
        </div>
      </div>
    </form>
  </section>
</template>
