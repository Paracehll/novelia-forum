<script setup lang="ts">
import { ArrowBackOutlined } from '@vicons/material';
import { computed, nextTick, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { type Post, type PostComment } from '@/api';
import { usePostFavorite } from '@/composables/usePostFavorite';
import { XButton } from '@novelia/web-kit';
import { XAsyncContent } from '@novelia/web-kit';
import PostTagList from '@/components/PostTagList.vue';
import { useCategoryStore } from '@/stores/category';
import { useCommentPageQuery, useCommentStore } from '@/stores/comment';
import { usePostQuery, usePostStore } from '@/stores/post';
import { postListReturn } from '@/utils/postNavigation';

import CommentComposer from './CommentComposer.vue';
import CommentList from './CommentList.vue';
import PostActions from './PostActions.vue';
import PostContent from './PostContent.vue';

const COMMENT_PAGE_SIZE = 50;

const route = useRoute();
const router = useRouter();
const categoryStore = useCategoryStore();
const commentStore = useCommentStore();
const postStore = usePostStore();

const composingComment = ref(false);
const replyTo = ref<PostComment>();

const postId = computed(() => {
  const value = Number(route.params.id);
  return Number.isSafeInteger(value) && value > 0 ? value : 0;
});

const {
  post,
  loading: postLoading,
  error: postError,
  retry: loadPost,
} = usePostQuery(postId);
watch(
  post,
  (value) => {
    if (value) document.title = `${value.title} | Novelia Forum`;
  },
  { immediate: true },
);

// The browser's initial fragment scroll runs before an async post is rendered.
watch(
  [() => route.hash, () => post.value?.id, postLoading, postError],
  async ([hash]) => {
    if (!hash || !post.value || postLoading.value || postError.value) return;
    await nextTick();
    let id = hash.slice(1);
    try {
      id = decodeURIComponent(id);
    } catch {
      // A literal percent sign is also valid in a heading ID.
    }
    document.getElementById(id)?.scrollIntoView({ block: 'start' });
  },
  { immediate: true, flush: 'post' },
);

const commentPage = computed(() => {
  const value = Number(route.query.commentPage);
  return Number.isInteger(value) && value > 0 ? value : 1;
});

const {
  comments,
  total: commentRecordTotal,
  loading: commentsLoading,
  error: commentsError,
  refresh: loadComments,
  retry: retryComments,
} = useCommentPageQuery(postId, commentPage, COMMENT_PAGE_SIZE);

const commentTotalPages = computed(() =>
  Math.max(1, Math.ceil(commentRecordTotal.value / COMMENT_PAGE_SIZE)),
);

const category = computed(() =>
  categoryStore.categories.find((item) => item.id === post.value?.categoryId),
);

const {
  canFavorite,
  loading: favoriteLoading,
  toggle: toggleFavorite,
} = usePostFavorite(post);

function jumpToComments() {
  const target = document.getElementById('comments');
  if (!target) return;
  target.focus({ preventScroll: true });
  target.scrollIntoView({
    behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches
      ? 'auto'
      : 'smooth',
    block: 'start',
  });
}

function changeCommentPage(nextPage: number) {
  void router.push({
    name: 'post-detail',
    params: { id: postId.value },
    query: nextPage > 1 ? { commentPage: String(nextPage) } : undefined,
  });
  document.querySelector('#comments')?.scrollIntoView({ behavior: 'smooth' });
}

async function handleCommentCreated(comment: PostComment) {
  if (!post.value) return;
  const targetPage = commentStore.registerCreatedComment(
    comment,
    commentPage.value,
    COMMENT_PAGE_SIZE,
  );
  postStore.setPost({
    ...post.value,
    commentsCount: post.value.commentsCount + 1,
  });

  if (comment.rootId == null && commentPage.value !== targetPage) {
    await router.push({
      name: 'post-detail',
      params: { id: postId.value },
      query: { commentPage: String(targetPage) },
    });
  }
  composingComment.value = false;
  replyTo.value = undefined;
  await nextTick();
  document
    .querySelector(`#comment-${comment.id}`)
    ?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  void loadComments();
}

function startReply(comment: PostComment) {
  if (replyTo.value?.id === comment.id) {
    replyTo.value = undefined;
    return;
  }
  composingComment.value = false;
  replyTo.value = comment;
  void nextTick(() =>
    document
      .querySelector(`#comment-reply-composer-${comment.id}`)
      ?.scrollIntoView({ behavior: 'smooth', block: 'center' }),
  );
}

function handleCommentStatusChanged(id: number) {
  // Refresh the authoritative count for both hiding/deletion and restoration.
  // Deleting an already hidden comment must not decrement the count twice.
  void loadPost();
  if (replyTo.value?.id === id) replyTo.value = undefined;
}

function startComment() {
  replyTo.value = undefined;
  composingComment.value = !composingComment.value;
  if (!composingComment.value) return;
  void nextTick(() =>
    document
      .querySelector('#comment-composer')
      ?.scrollIntoView({ behavior: 'smooth', block: 'center' }),
  );
}

function editPost() {
  void router.push({ name: 'post-edit', params: { id: postId.value } });
}

function handlePostUpdated(
  patch: Partial<Pick<Post, 'pinOrder' | 'commentsLocked'>>,
) {
  if (post.value) postStore.setPost({ ...post.value, ...patch });
}

async function handleAuthorCommentsDeleted() {
  replyTo.value = undefined;
  await Promise.all([loadPost(), retryComments()]);
}

function leaveDeletedPost() {
  const slug = category.value?.slug ?? categoryStore.defaultCategory.slug;
  postStore.removePost(postId.value);
  void router.replace({
    name: 'posts',
    params: {
      slug,
    },
  });
}

function returnToList() {
  if (postListReturn) {
    const { path, query, hash } = router.resolve(postListReturn.path);
    void router.push({
      path,
      query,
      hash,
      state: { postListScrollTop: postListReturn.scrollTop },
    });
    return;
  }
  void router.push({
    name: 'posts',
    params: {
      slug: category.value?.slug ?? categoryStore.defaultCategory.slug,
    },
  });
}
</script>

<template>
  <div class="page-container py-4 md:py-6">
    <div class="mx-auto max-w-4xl">
      <div class="mb-3 flex flex-wrap items-center gap-1.5">
        <XButton
          variant="plain"
          size="none"
          class="min-h-9 rounded-sm text-xs font-medium text-primary hover:text-primary-hover"
          title="返回列表"
          aria-label="返回列表"
          @click="returnToList"
        >
          <ArrowBackOutlined class="size-4" aria-hidden="true" />
          {{ post && !postError ? (category?.title ?? '未分类') : '返回列表' }}
        </XButton>
        <PostTagList
          v-if="post && !postLoading && !postError"
          class="min-w-0"
          :tags="post.tags"
          :pinned="post.pinOrder != null"
          :locked="post.commentsLocked"
        />
        <div
          v-else-if="postLoading"
          class="h-3 w-24 animate-pulse rounded-sm bg-divider"
        />
      </div>
      <XAsyncContent
        :loading="postLoading"
        :error="postError"
        size="large"
        heading-tag="h1"
        error-title="帖子加载失败"
        @retry="loadPost"
      >
        <template #loading>
          <div class="py-6">
            <div class="mt-4 h-8 w-4/5 animate-pulse rounded-sm bg-border" />
            <div class="mt-5 h-4 w-56 animate-pulse rounded-sm bg-divider" />
            <div class="my-6 h-px bg-divider" />
            <div class="h-4 w-full animate-pulse rounded-sm bg-divider" />
            <div class="mt-3 h-4 w-full animate-pulse rounded-sm bg-divider" />
            <div class="mt-3 h-4 w-2/3 animate-pulse rounded-sm bg-divider" />
          </div>
        </template>

        <template v-if="post">
          <PostContent :post="post">
            <template #actions>
              <PostActions
                :key="post.id"
                :post="post"
                :can-favorite="canFavorite"
                :favorite-loading="favoriteLoading"
                @favorite="toggleFavorite"
                @comments="jumpToComments"
                @edit="editPost"
                @deleted="leaveDeletedPost"
                @updated="handlePostUpdated"
                @author-comments-deleted="handleAuthorCommentsDeleted"
              />
            </template>
          </PostContent>
          <div id="comments" tabindex="-1" class="scroll-mt-20">
            <CommentList
              :comments="comments"
              :loading="commentsLoading"
              :error="commentsError"
              :page="commentPage"
              :comments-count="post.commentsCount"
              :total-pages="commentTotalPages"
              :locked="post.commentsLocked"
              :post-id="post.id"
              :composing="composingComment"
              :reply-to-id="replyTo?.id"
              @retry="retryComments"
              @comment="startComment"
              @change-page="changeCommentPage"
              @reply="startReply"
              @cancel-reply="replyTo = undefined"
              @created="handleCommentCreated"
              @status-changed="handleCommentStatusChanged"
              @author-comments-deleted="handleAuthorCommentsDeleted"
            >
              <template #composer>
                <CommentComposer
                  v-if="composingComment"
                  id="comment-composer"
                  :post-id="post.id"
                  :locked="post.commentsLocked"
                  @created="handleCommentCreated"
                />
              </template>
            </CommentList>
          </div>
        </template>
      </XAsyncContent>
    </div>
  </div>
</template>
