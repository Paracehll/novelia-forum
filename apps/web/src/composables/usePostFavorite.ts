import {
  computed,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue';
import { getApiErrorMessage, Notify, useWebKit } from '@novelia/web-kit';

import { setPostFavorite, type Post } from '@/api';
import { usePostStore } from '@/stores/post';

// Create once per detail page; both favorite buttons share this request lock.
export function usePostFavorite(post: MaybeRefOrGetter<Post | undefined>) {
  const { whoami } = useWebKit();
  const postStore = usePostStore();
  const loading = ref(false);
  const canFavorite = computed(() => whoami.value.isSignedIn);
  let context = 0;

  // Cancel in-flight work when the post or the signed-in account changes. The
  // account id (not `isSignedIn`) is what distinguishes one viewer from another,
  // and favoriting has no admin-mode variant, so the mode must not reset the lock.
  watch(
    () => [toValue(post)?.id, whoami.value.user?.id],
    (next, previous) => {
      if (next.some((value, index) => value !== previous[index])) {
        context++;
        loading.value = false;
      }
    },
    { flush: 'sync' },
  );
  onScopeDispose(() => context++);

  async function toggle() {
    const currentPost = toValue(post);
    if (!currentPost || !canFavorite.value || loading.value) return;
    const requestContext = context;
    const nextValue = !currentPost.favorited;
    loading.value = true;
    try {
      await setPostFavorite(currentPost.id, nextValue);
      const latestPost = toValue(post);
      if (requestContext !== context || !latestPost) return;
      // Merge into the latest data, preserving concurrent comment/management changes.
      postStore.setPost({ ...latestPost, favorited: nextValue });
      Notify.success(nextValue ? '帖子已收藏' : '已取消收藏');
    } catch (reason) {
      if (requestContext !== context) return;
      const message = await getApiErrorMessage(reason, '更新收藏失败');
      if (requestContext === context) Notify.error(message);
    } finally {
      if (requestContext === context) loading.value = false;
    }
  }

  return { canFavorite, loading, toggle };
}
