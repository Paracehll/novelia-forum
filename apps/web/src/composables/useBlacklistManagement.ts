import { onScopeDispose, ref, watch } from 'vue';
import { getApiErrorMessage, Notify, useWebKit } from '@novelia/web-kit';
import { getBlacklist, setUserBlocked, type BlacklistEntry } from '@/api';
import { usePostStore } from '@/stores/post';
import { useCommentStore } from '@/stores/comment';

export function useBlacklistManagement() {
  const { whoami } = useWebKit();
  const posts = usePostStore();
  const comments = useCommentStore();
  const entries = ref<BlacklistEntry[]>([]);
  const loading = ref(false);
  const error = ref('');
  const selected = ref<BlacklistEntry>();
  const removing = ref(false);
  let active = true;
  let context = 0;
  let loadVersion = 0;
  let controller: AbortController | undefined;

  async function load() {
    controller?.abort();
    const version = ++loadVersion;
    const viewerId = whoami.value.user?.id;
    error.value = '';
    if (!active || !viewerId) {
      loading.value = false;
      return;
    }
    const request = new AbortController();
    controller = request;
    const isCurrent = () =>
      active && version === loadVersion && whoami.value.user?.id === viewerId;
    loading.value = true;
    try {
      const result = await getBlacklist(request.signal);
      if (isCurrent()) entries.value = result.items;
    } catch (reason) {
      if (!isCurrent() || request.signal.aborted) return;
      const message = await getApiErrorMessage(reason, '加载黑名单失败');
      if (isCurrent()) error.value = message;
    } finally {
      if (isCurrent()) loading.value = false;
    }
  }

  watch(
    () => whoami.value.user?.id,
    () => {
      context++;
      entries.value = [];
      selected.value = undefined;
      removing.value = false;
      void load();
    },
    { immediate: true, flush: 'sync' },
  );

  async function confirmRemove() {
    const entry = selected.value;
    const viewerId = whoami.value.user?.id;
    if (!active || removing.value || !entry || !viewerId) return;
    const requestContext = context;
    const isCurrent = () =>
      requestContext === context && whoami.value.user?.id === viewerId;
    removing.value = true;
    try {
      await setUserBlocked(entry.userId, false);
      if (!isCurrent()) return;
      // Update content caches even when navigation has closed this page.
      posts.refreshBlacklist(entry.userId, false);
      comments.refreshBlacklist(entry.userId, false);
      if (!active) return;
      // Prevent an older list response from restoring the removed entry.
      controller?.abort();
      loadVersion++;
      loading.value = false;
      error.value = '';
      entries.value = entries.value.filter(
        (item) => item.userId !== entry.userId,
      );
      selected.value = undefined;
      Notify.success('已取消拉黑');
    } catch (reason) {
      if (!active || !isCurrent()) return;
      const message = await getApiErrorMessage(reason, '取消拉黑失败');
      if (active && isCurrent()) Notify.error(message);
    } finally {
      if (active && isCurrent()) removing.value = false;
    }
  }

  onScopeDispose(() => {
    active = false;
    controller?.abort();
  });

  return {
    whoami,
    entries,
    loading,
    error,
    selected,
    removing,
    load,
    confirmRemove,
  };
}
