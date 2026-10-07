<script setup lang="ts">
import { SearchOutlined } from '@vicons/material';
import { computed, ref, watch } from 'vue';

import { type PostSort } from '@/api';
import { XSelect } from '@novelia/web-kit';
import { useCategoryStore } from '@/stores/category';

const props = defineProps<{
  categoryId: number;
  query: string;
  tagId?: number;
  sort: PostSort;
}>();

const emit = defineEmits<{
  apply: [filters: { query: string; tagId?: number; sort: PostSort }];
}>();
const categoryStore = useCategoryStore();

const ALL_TAGS = 'all';

const queryInput = ref(props.query);
const tagInput = ref(props.tagId ? String(props.tagId) : ALL_TAGS);
const sortInput = ref<PostSort>(props.sort);
const tags = computed(() => categoryStore.tagsByCategoryId(props.categoryId));
const tagOptions = computed(() => [
  { label: '全部标签', value: ALL_TAGS },
  ...tags.value.map((tag) => ({ label: tag.name, value: String(tag.id) })),
]);
const sortOptions = [
  { label: '最近活跃', value: 'active' },
  { label: '最新发布', value: 'newest' },
  { label: '浏览最多', value: 'views' },
  { label: '评论最多', value: 'comments' },
];

function syncInputs() {
  queryInput.value = props.query;
  tagInput.value = props.tagId ? String(props.tagId) : ALL_TAGS;
  sortInput.value = props.sort;
}

function apply() {
  const tagId = Number(tagInput.value);
  emit('apply', {
    query: queryInput.value.trim(),
    tagId: Number.isSafeInteger(tagId) && tagId > 0 ? tagId : undefined,
    sort: sortInput.value,
  });
}

watch(() => [props.query, props.tagId, props.sort], syncInputs);
watch(
  () => props.categoryId,
  (_categoryId, previousCategoryId) => {
    if (previousCategoryId != null) tagInput.value = ALL_TAGS;
  },
);
</script>

<template>
  <form
    class="flex min-w-0 flex-wrap gap-2 sm:flex-nowrap"
    role="search"
    @submit.prevent="apply"
  >
    <label class="relative w-full min-w-0 sm:flex-1">
      <span class="sr-only">搜索帖子</span>
      <SearchOutlined
        class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted"
        aria-hidden="true"
      />
      <input
        v-model="queryInput"
        type="search"
        enterkeyhint="search"
        class="min-h-10 w-full rounded-sm border border-border bg-transparent pr-3 pl-9 text-sm outline-none transition placeholder:text-muted/70 focus:border-primary focus:ring-2 focus:ring-primary/15 sm:pr-10"
        placeholder="搜索标题或正文"
      />
      <span
        class="pointer-events-none absolute top-1/2 right-3 hidden -translate-y-1/2 text-sm text-muted/70 sm:block"
        aria-hidden="true"
      >
        ↵
      </span>
    </label>

    <XSelect
      v-if="tags.length"
      v-model="tagInput"
      :options="tagOptions"
      aria-label="按标签过滤"
      class="min-w-0 flex-1 sm:w-32 sm:flex-none"
      @change="apply"
    />

    <XSelect
      v-model="sortInput"
      :options="sortOptions"
      aria-label="帖子排序"
      class="min-w-0 flex-1 sm:w-32 sm:flex-none"
      @change="apply"
    />
  </form>
</template>
