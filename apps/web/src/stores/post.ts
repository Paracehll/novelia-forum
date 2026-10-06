import { computed, ref, watch, type MaybeRefOrGetter, toValue } from 'vue';
import { defineStore } from 'pinia';
import { useQuery, useQueryCache } from '@pinia/colada';

import {
  getFavoritePosts,
  getMyPosts,
  getPost,
  getPosts,
  type Page,
  type Post,
  type PostSummary,
} from '@/api';
import { whoami } from '@/session';

// Post payloads vary by viewer only through `favorited`, which the backend
// derives from the account id; admin mode cannot change a post response, so
// keying on it would needlessly drop the cache on every mode toggle.
const viewerKey = computed(() => whoami.value.user?.id ?? 'guest');
const postKey = (id: number) => ['posts', viewerKey.value, 'detail', id];
const listKey = () => ['posts', viewerKey.value, 'list'];

function isUnavailable(error: unknown) {
  const status = (error as { response?: { status?: number } } | null)?.response
    ?.status;
  return status === 401 || status === 403 || status === 404 || status === 410;
}

export const usePostStore = defineStore('post', () => {
  const cache = useQueryCache();
  const currentPostId = ref<number>();
  const currentPost = computed(() =>
    currentPostId.value == null
      ? undefined
      : cache.getQueryData<Post>(postKey(currentPostId.value)),
  );

  watch(
    viewerKey,
    (_, previous) => {
      const filter = { key: ['posts', previous] };
      cache.cancelQueries(filter);
      for (const entry of cache.getEntries(filter)) cache.remove(entry);
    },
    { flush: 'sync' },
  );

  function invalidateLists() {
    cache.cancelQueries({ key: listKey() });
    void cache.invalidateQueries({ key: listKey() });
  }

  function setPost(post: Post) {
    cache.cancelQueries({ key: postKey(post.id), exact: true });
    cache.setQueryData<Post>(postKey(post.id), post);
    // Update visible copies immediately; membership and ordering come from the server.
    cache.cancelQueries({ key: listKey() });
    for (const entry of cache.getEntries({ key: listKey() })) {
      const page = entry.state.value.data as Page<PostSummary> | undefined;
      if (page)
        cache.setQueryData<Page<PostSummary>>(entry.key, {
          ...page,
          items: page.items.map((item) => (item.id === post.id ? post : item)),
        });
    }
    invalidateLists();
  }

  function removePost(id: number) {
    const filter = { key: postKey(id), exact: true };
    cache.cancelQueries(filter);
    // Keep active observers attached, but discard the inaccessible content.
    for (const entry of cache.getEntries(filter)) {
      cache.setEntryState(entry, {
        status: 'error',
        data: undefined,
        error: new Error('帖子已删除或不可访问'),
      });
      cache.invalidate(entry);
    }
    cache.cancelQueries({ key: listKey() });
    for (const entry of cache.getEntries({ key: listKey() })) {
      const page = entry.state.value.data as Page<PostSummary> | undefined;
      if (page?.items.some((item) => item.id === id)) {
        cache.setQueryData<Page<PostSummary>>(entry.key, {
          total: Math.max(0, page.total - 1),
          items: page.items.filter((item) => item.id !== id),
        });
      }
    }
    invalidateLists();
  }

  return { currentPostId, currentPost, setPost, removePost, invalidateLists };
});

type CategoryParams = Parameters<typeof getPosts>[0];
type ListKind = 'category' | 'mine' | 'favorites';

export function usePostListQuery(
  kind: ListKind,
  params: MaybeRefOrGetter<CategoryParams>,
) {
  usePostStore();
  const query = useQuery({
    key: () => [...listKey(), kind, toValue(params)],
    enabled: () => kind === 'category' || whoami.value.isSignedIn,
    query: ({ signal }) => {
      const value = toValue(params);
      return kind === 'category'
        ? getPosts(value, signal)
        : kind === 'mine'
          ? getMyPosts(value, signal)
          : getFavoritePosts(value, signal);
    },
  });
  return {
    posts: computed(() => query.data.value?.items ?? []),
    total: computed(() => query.data.value?.total ?? 0),
    loading: query.isPending,
    error: computed(() =>
      !query.data.value && query.error.value ? query.error.value.message : '',
    ),
    retry: () => query.refetch(),
  };
}

export function usePostQuery(
  id: MaybeRefOrGetter<number>,
  options: { editing?: boolean; enabled?: MaybeRefOrGetter<boolean> } = {},
) {
  const store = usePostStore();
  const cache = useQueryCache();
  const query = useQuery({
    key: () => postKey(toValue(id)),
    enabled: () =>
      !!toValue(id) && (options.enabled == null || toValue(options.enabled)),
    staleTime: options.editing ? 0 : 60_000,
    // Refresh on editor entry; the editor protects any unsaved local draft.
    refetchOnMount: options.editing ? 'always' : true,
    refetchOnWindowFocus: !options.editing,
    refetchOnReconnect: !options.editing,
    query: async ({ signal }) => {
      const requestedId = toValue(id);
      const key = postKey(requestedId);
      try {
        return await getPost(requestedId, signal);
      } catch (error) {
        if (!signal.aborted && isUnavailable(error)) {
          for (const entry of cache.getEntries({ key, exact: true })) {
            cache.setEntryState(entry, {
              status: 'error',
              data: undefined,
              error,
            });
          }
        }
        throw error;
      }
    },
  });
  watch(
    () => toValue(id),
    (value) => {
      store.currentPostId = value || undefined;
    },
    { immediate: true },
  );
  return {
    post: query.data,
    loading: computed(() => !!toValue(id) && query.isPending.value),
    error: computed(() =>
      !toValue(id)
        ? '帖子地址无效'
        : !query.data.value && query.error.value
          ? query.error.value.message
          : '',
    ),
    retry: () => query.refetch(),
    refresh: () => query.refresh(),
  };
}
