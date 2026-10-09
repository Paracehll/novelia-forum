import { computed, toValue, watch, type MaybeRefOrGetter } from 'vue';
import { defineStore } from 'pinia';
import { useQuery, useQueryCache, type UseQueryEntry } from '@pinia/colada';

import {
  createPostComment,
  deletePostComment,
  getPostCommentReplies,
  getPostComments,
  setPostCommentStatus,
  updatePostComment,
  type CommentStatus,
  type Page,
  type PostComment,
} from '@/api';
import { whoami } from '@/session';

export const COMMENT_REPLY_PAGE_SIZE = 20;
const CACHE_MAX_PAGES = 30;
// Keep each account's cache stable when toggling admin mode.
const viewerKey = computed(() => whoami.value.user?.id ?? 'guest');
const commentsKey = () => ['comments', viewerKey.value];
const pageKey = (postId: number, page: number, pageSize: number) => [
  ...commentsKey(),
  postId,
  'roots',
  page,
  pageSize,
];
const repliesKey = (
  postId: number,
  rootId: number,
  page: number,
  pageSize: number,
) => [...commentsKey(), postId, 'replies', rootId, page, pageSize];

export const useCommentStore = defineStore('comment', () => {
  const cache = useQueryCache();
  const accessedAt = new WeakMap<UseQueryEntry, number>();

  watch(
    viewerKey,
    (_, previous) => {
      const filter = { key: ['comments', previous] };
      cache.cancelQueries(filter);
      for (const entry of cache.getEntries(filter)) cache.remove(entry);
    },
    { flush: 'sync' },
  );

  function touchPage(postId: number, page: number, pageSize: number) {
    const key = pageKey(postId, page, pageSize);
    const retained = new Set(cache.getEntries({ key, exact: true }));
    for (const entry of retained) accessedAt.set(entry, Date.now());
    const entries = cache.getEntries({ key: commentsKey() });
    if (entries.length <= CACHE_MAX_PAGES) return;
    const candidates = entries
      .filter(
        (entry) => !retained.has(entry) && !entry.active && !entry.pending,
      )
      .sort(
        (a, b) => (accessedAt.get(a) ?? a.when) - (accessedAt.get(b) ?? b.when),
      );
    for (const entry of candidates.slice(0, entries.length - CACHE_MAX_PAGES))
      cache.remove(entry);
  }

  // Cancel old responses before updating every cached copy of a comment. This also
  // covers pending pages whose contents are not yet known.
  function updateCachedComments(
    update: (comment: PostComment) => PostComment,
    postId?: number,
  ) {
    const filter = {
      key: postId == null ? commentsKey() : [...commentsKey(), postId],
    };
    cache.cancelQueries(filter);
    for (const entry of cache.getEntries(filter)) {
      const page = entry.state.value.data as Page<PostComment> | undefined;
      if (page)
        cache.setQueryData<Page<PostComment>>(entry.key, {
          ...page,
          items: page.items.map(update),
        });
    }
    void cache.invalidateQueries(filter);
  }

  function refreshBlacklist(authorId: number, blocked: boolean) {
    const filter = { key: commentsKey() };
    cache.cancelQueries(filter);
    if (blocked) {
      for (const entry of cache.getEntries(filter)) {
        const page = entry.state.value.data as Page<PostComment> | undefined;
        if (!page) continue;
        const items = page.items.filter((item) => item.authorId !== authorId);
        cache.setQueryData<Page<PostComment>>(entry.key, {
          total: Math.max(0, page.total - (page.items.length - items.length)),
          items,
        });
      }
    }
    void cache.invalidateQueries(filter);
  }

  async function createComment(
    postId: number,
    input: { content: string; rootId?: number },
  ) {
    const viewer = viewerKey.value;
    const comment = await createPostComment(postId, input);
    if (viewer === viewerKey.value) {
      // The caller inserts the response before revalidating pagination.
      void cache.invalidateQueries({ key: [...commentsKey(), postId] }, false);
    }
    return comment;
  }

  function registerCreatedComment(
    comment: PostComment,
    currentPage: number,
    pageSize: number,
  ) {
    const filter = { key: [...commentsKey(), comment.postId] };
    cache.cancelQueries(filter);
    const key = pageKey(comment.postId, currentPage, pageSize);
    const current = cache.getQueryData<Page<PostComment>>(key) ?? {
      items: [],
      total: 0,
    };
    const entries = cache.getEntries(filter);
    const rootEntries = entries.filter((entry) => entry.key.includes('roots'));
    const alreadyIncluded = entries.some((entry) =>
      (entry.state.value.data as Page<PostComment> | undefined)?.items.some(
        (item) => item.id === comment.id,
      ),
    );
    if (comment.rootId != null) {
      for (const entry of rootEntries) {
        const page = entry.state.value.data as Page<PostComment> | undefined;
        if (!page) continue;
        if (entry.key.includes('roots')) {
          cache.setQueryData<Page<PostComment>>(entry.key, {
            ...page,
            items: page.items.map((item) =>
              item.id === comment.rootId
                ? { ...item, replyCount: item.replyCount + 1 }
                : item,
            ),
          });
        }
      }
      void cache.invalidateQueries({
        key: [...commentsKey(), comment.postId, 'replies', comment.rootId],
      });
      return Math.max(1, Math.ceil(current.total / pageSize));
    }
    for (const entry of rootEntries) {
      const page = entry.state.value.data as Page<PostComment> | undefined;
      if (page)
        cache.setQueryData<Page<PostComment>>(entry.key, {
          ...page,
          total: page.total + (alreadyIncluded ? 0 : 1),
        });
    }
    const items = [...current.items];
    if (!items.some((item) => item.id === comment.id)) {
      if (currentPage === 1) items.unshift(comment);
    }
    cache.setQueryData<Page<PostComment>>(key, {
      items: items.slice(0, pageSize),
      total: current.total + (alreadyIncluded ? 0 : 1),
    });
    void cache.invalidateQueries(filter, false);
    return 1;
  }

  async function updateComment(id: number, content: string) {
    const viewer = viewerKey.value;
    const comment = await updatePostComment(id, content);
    if (viewer === viewerKey.value) {
      updateCachedComments(
        (item) =>
          item.id === id ? { ...comment, replyCount: item.replyCount } : item,
        comment.postId,
      );
    }
    return comment;
  }

  function applyStatus(id: number, status: CommentStatus) {
    updateCachedComments((comment) =>
      comment.id === id
        ? {
            ...comment,
            status,
            content: whoami.value.isAdmin ? comment.content : '',
          }
        : comment,
    );
  }

  function registerDeletedCommentsByAuthor(authorId: number) {
    updateCachedComments((comment) =>
      comment.authorId === authorId
        ? {
            ...comment,
            status: 2,
            content: whoami.value.isAdmin ? comment.content : '',
          }
        : comment,
    );
  }

  async function deleteComment(id: number, asAdmin: boolean) {
    const viewer = viewerKey.value;
    if (asAdmin) await setPostCommentStatus(id, 'deleted');
    else await deletePostComment(id);
    if (viewer === viewerKey.value) applyStatus(id, 2);
  }

  async function hideComment(id: number) {
    const viewer = viewerKey.value;
    await setPostCommentStatus(id, 'hidden');
    if (viewer === viewerKey.value) applyStatus(id, 1);
  }

  async function unhideComment(id: number) {
    const viewer = viewerKey.value;
    await setPostCommentStatus(id, 'published');
    if (viewer === viewerKey.value) applyStatus(id, 0);
  }

  return {
    createComment,
    deleteComment,
    hideComment,
    unhideComment,
    registerCreatedComment,
    registerDeletedCommentsByAuthor,
    updateComment,
    touchPage,
    refreshBlacklist,
  };
});

export function useCommentPageQuery(
  postId: MaybeRefOrGetter<number>,
  page: MaybeRefOrGetter<number>,
  pageSize: number,
) {
  const store = useCommentStore();
  const cache = useQueryCache();
  const query = useQuery({
    key: () => pageKey(toValue(postId), toValue(page), pageSize),
    enabled: () => !!toValue(postId),
    staleTime: 60_000,
    gcTime: 5 * 60_000,
    query: async ({ signal, entry }) => {
      try {
        const requestedPostId = toValue(postId);
        const viewer = viewerKey.value;
        const result = await getPostComments(
          requestedPostId,
          { page: toValue(page), pageSize },
          signal,
        );
        // Seed before mounting threads so their first page needs no extra request.
        // Keep one cache copy of replies, shared by edits and moderation updates.
        const items = result.items.map(({ replies, ...comment }) => {
          if (replies && !signal.aborted && viewer === viewerKey.value) {
            const key = repliesKey(
              requestedPostId,
              comment.id,
              1,
              COMMENT_REPLY_PAGE_SIZE,
            );
            cache.cancelQueries({ key, exact: true });
            cache.setQueryData<Page<PostComment>>(key, replies);
          }
          return comment;
        });
        return { ...result, items };
      } catch (error) {
        const failure =
          error instanceof Error ? error : new Error('无法加载评论');
        const status = (error as { response?: { status?: number } } | null)
          ?.response?.status;
        if (
          !signal.aborted &&
          status != null &&
          [401, 403, 404, 410].includes(status)
        ) {
          cache.setEntryState(entry, {
            status: 'error',
            data: undefined,
            error: failure,
          });
        }
        throw failure;
      }
    },
  });
  watch(
    [() => toValue(postId), () => toValue(page), query.asyncStatus],
    () => {
      store.touchPage(toValue(postId), toValue(page), pageSize);
    },
    { immediate: true, flush: 'post' },
  );
  return {
    comments: computed(() => query.data.value?.items ?? []),
    total: computed(() => query.data.value?.total ?? 0),
    loading: computed(() => !!toValue(postId) && query.isPending.value),
    error: computed(() =>
      !query.data.value && query.error.value ? query.error.value.message : '',
    ),
    refresh: () => query.refresh(),
    retry: () => query.refetch(),
  };
}

export function useCommentReplyPageQuery(
  postId: MaybeRefOrGetter<number>,
  rootId: MaybeRefOrGetter<number>,
  page: MaybeRefOrGetter<number>,
  pageSize: number,
  enabled: MaybeRefOrGetter<boolean>,
) {
  const query = useQuery({
    key: () =>
      repliesKey(toValue(postId), toValue(rootId), toValue(page), pageSize),
    enabled: () => toValue(enabled) && !!toValue(postId) && !!toValue(rootId),
    staleTime: 60_000,
    gcTime: 5 * 60_000,
    query: ({ signal }) =>
      getPostCommentReplies(
        toValue(postId),
        toValue(rootId),
        { page: toValue(page), pageSize },
        signal,
      ),
  });
  return {
    replies: computed(() => query.data.value?.items ?? []),
    total: computed(() => query.data.value?.total ?? 0),
    loading: computed(() => toValue(enabled) && query.isPending.value),
    error: computed(() =>
      !query.data.value && query.error.value ? query.error.value.message : '',
    ),
    refresh: () => query.refresh(),
    retry: () => query.refetch(),
  };
}
