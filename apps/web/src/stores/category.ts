import { computed } from 'vue';
import { defineStore } from 'pinia';
import { useQuery } from '@pinia/colada';

import { authUser, getCategories, type CategoryListItem } from '@/api';

const CACHE_MAX_AGE = 15 * 60 * 1000;
const LAST_CATEGORY_KEY = 'forum:last-category';

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

function getStoredCategorySlug(): string | null {
  try {
    return localStorage.getItem(LAST_CATEGORY_KEY);
  } catch {
    return null;
  }
}

function setStoredCategorySlug(slug: string): void {
  try {
    localStorage.setItem(LAST_CATEGORY_KEY, slug);
  } catch {
    // Ignore storage errors
  }
}

export const useCategoryStore = defineStore('category', () => {
  const query = useQuery({
    key: ['categories'],
    staleTime: CACHE_MAX_AGE,
    query: async ({ signal }) => {
      const items = await getCategories(signal);
      if (!items.length) throw new Error('分类列表为空');
      return items;
    },
  });

  const items = computed(() => query.data.value ?? []);
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

  function saveLastVisitedCategory(slug: string) {
    if (categories.value.some((category) => category.slug === slug)) {
      setStoredCategorySlug(slug);
    }
  }

  function getLastVisitedCategory(): string {
    const stored = getStoredCategorySlug();
    if (
      stored &&
      categories.value.some((category) => category.slug === stored)
    ) {
      return stored;
    }
    return (
      categories.value.find((category) => category.slug === 'announcements')
        ?.slug ?? defaultCategory.value.slug
    );
  }

  return {
    categories,
    writableCategories,
    canPublish,
    defaultCategory,
    items,
    tagsByCategoryId,
    initialize,
    saveLastVisitedCategory,
    getLastVisitedCategory,
    loading: query.isLoading,
    error: computed(() => query.error.value?.message ?? ''),
    refresh: () => query.refetch(),
  };
});
