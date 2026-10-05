import { computed, watch } from 'vue';
import { defineStore } from 'pinia';
import { useQuery, useQueryCache } from '@pinia/colada';

import { authUser, getCategories, type CategoryListItem } from '@/api';

const CACHE_MAX_AGE = 15 * 60 * 1000;
const CATEGORY_TITLES: Record<string, string> = {
  novel: '小说讨论',
  announcements: '站务公告',
  feedback: '意见反馈',
};
const FALLBACK_CATEGORIES: CategoryListItem[] = [
  { id: 1, slug: 'announcements', tags: [] },
  { id: 2, slug: 'feedback', tags: [] },
  { id: 100, slug: 'novel', tags: [] },
];

const QUERY_KEY = ['categories'];
const STORAGE_KEY = 'forum:categories:v1';
interface CategorySnapshot {
  items: CategoryListItem[];
  fetchedAt: number;
}

function readSnapshot(): CategorySnapshot | undefined {
  try {
    const value = JSON.parse(
      localStorage.getItem(STORAGE_KEY) ?? 'null',
    ) as CategorySnapshot | null;
    if (
      !value ||
      !Array.isArray(value.items) ||
      !value.items.length ||
      !Number.isFinite(value.fetchedAt) ||
      value.fetchedAt <= 0 ||
      value.fetchedAt > Date.now()
    )
      return;
    if (
      !value.items.every(
        (item) =>
          item &&
          Number.isSafeInteger(item.id) &&
          item.id > 0 &&
          typeof item.slug === 'string' &&
          Array.isArray(item.tags) &&
          item.tags.every(
            (tag) =>
              tag &&
              Number.isSafeInteger(tag.id) &&
              typeof tag.name === 'string' &&
              Number.isFinite(tag.color) &&
              Number.isFinite(tag.sortOrder),
          ),
      )
    )
      return;
    return value;
  } catch {
    // Storage is optional; fall back to the network and built-in navigation.
  }
}

export const useCategoryStore = defineStore('category', () => {
  const cache = useQueryCache();
  const saved = readSnapshot();
  if (saved && !cache.getQueryData(QUERY_KEY)) {
    cache.setQueryData<CategorySnapshot>(QUERY_KEY, saved);
    // Hydration must not make an old snapshot fresh again.
    for (const entry of cache.getEntries({ key: QUERY_KEY, exact: true })) {
      entry.when = saved.fetchedAt;
    }
  }
  const query = useQuery({
    key: QUERY_KEY,
    staleTime: CACHE_MAX_AGE,
    query: async ({ signal }) => {
      const items = await getCategories(signal);
      if (!items.length) throw new Error('分类列表为空');
      return { items, fetchedAt: Date.now() };
    },
  });
  watch(query.data, (value) => {
    if (!value) return;
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
    } catch {
      // Keep the in-memory query usable when storage is unavailable or full.
    }
  });

  const items = computed(() => query.data.value?.items ?? []);
  const fetchedAt = computed(() => query.data.value?.fetchedAt ?? 0);
  const categories = computed(() =>
    (items.value.length ? items.value : FALLBACK_CATEGORIES)
      .map((category) => ({
        ...category,
        title: CATEGORY_TITLES[category.slug] ?? category.slug,
      }))
      .sort((left, right) => {
        const order = ['announcements', 'novel', 'feedback'];
        return order.indexOf(left.slug) - order.indexOf(right.slug);
      }),
  );
  const defaultCategory = computed(
    () =>
      categories.value.find((category) => category.slug === 'novel') ??
      categories.value[0],
  );
  const writableCategories = computed(() =>
    categories.value.filter((category) => canPublish(category.slug)),
  );

  function canPublish(slug: string) {
    return slug !== 'announcements' || authUser.value?.role === 'admin';
  }

  function tagsByCategoryId(categoryId: number) {
    return (
      categories.value.find((category) => category.id === categoryId)?.tags ??
      []
    );
  }

  async function initialize() {
    const pending = query.refresh();
    if (!items.value.length) await pending;
  }

  return {
    categories,
    writableCategories,
    canPublish,
    defaultCategory,
    items,
    fetchedAt,
    tagsByCategoryId,
    initialize,
    loading: query.isLoading,
    error: computed(() => query.error.value?.message ?? ''),
    refresh: () => query.refetch(),
  };
});
