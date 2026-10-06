<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { createPost } from '@/api';
import { usePostStore } from '@/stores/post';
import {
  getApiErrorMessage,
  Notify,
  useWebKit,
  XAsyncContent,
} from '@novelia/web-kit';
import { useCategoryStore } from '@/stores/category';
import { useDraftStore } from '@/stores/draft';
import PostPublishingNotice from './PostPublishingNotice.vue';

import PostForm from './PostForm.vue';

const { whoami } = useWebKit();
const postStore = usePostStore();
const route = useRoute();
const router = useRouter();
const categoryStore = useCategoryStore();
const draftStore = useDraftStore();
const initialCategory =
  categoryStore.writableCategories.find(
    (category) => category.slug === route.query.category,
  )?.slug ?? categoryStore.defaultCategory.slug;
const title = ref('');
const categorySlug = ref(initialCategory);
const selectedTagIds = ref<number[]>([]);
const content = ref('');
const submitting = ref(false);

const selectedCategory = computed(
  () =>
    categoryStore.writableCategories.find(
      (category) => category.slug === categorySlug.value,
    ) ?? categoryStore.defaultCategory,
);
const tags = computed(() => selectedCategory.value.tags);
const draftUserId = computed(() => whoami.value.user?.id ?? 0);
const categoriesReady = computed(() => categoryStore.items.length > 0);

watch(
  [draftUserId, categoriesReady],
  ([userId, ready]) => {
    title.value = '';
    categorySlug.value = initialCategory;
    selectedTagIds.value = [];
    content.value = '';
    if (!userId || !ready) return;
    const draft = draftStore.getPostDraft(userId);
    if (!draft) return;
    const categoryItem =
      categoryStore.writableCategories.find(
        (item) => item.slug === draft.category,
      ) ?? categoryStore.defaultCategory;
    const validIds = new Set(categoryItem.tags.map((tag) => tag.id));
    title.value = draft.title;
    categorySlug.value = categoryItem.slug;
    selectedTagIds.value = draft.tagIds.filter((id) => validIds.has(id));
    content.value = draft.content;
  },
  { immediate: true },
);

watch(
  [title, categorySlug, selectedTagIds, content],
  () => {
    if (!draftUserId.value || !categoriesReady.value) return;
    draftStore.savePostDraft(draftUserId.value, {
      title: title.value,
      category: categorySlug.value,
      tagIds: selectedTagIds.value,
      content: content.value,
    });
  },
  { deep: true },
);

function changeCategory() {
  selectedTagIds.value = [];
}

async function submitPost() {
  if (!whoami.value.isSignedIn || !categoriesReady.value || submitting.value)
    return;
  submitting.value = true;
  try {
    const post = await createPost({
      categoryId: selectedCategory.value.id,
      title: title.value.trim(),
      content: content.value,
      tagIds: selectedTagIds.value,
    });
    postStore.setPost(post);
    draftStore.clearPostDraft(draftUserId.value);
    title.value = '';
    content.value = '';
    selectedTagIds.value = [];
    Notify.success('帖子已发布');
    await router.replace({ name: 'post-detail', params: { id: post.id } });
  } catch (error) {
    Notify.error(await getApiErrorMessage(error, '帖子发布失败'));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="page-container py-4 md:py-6">
    <div class="mx-auto max-w-4xl">
      <section>
        <h1 class="text-xl font-bold text-ink">发表帖子</h1>

        <div v-if="!whoami.isSignedIn" class="mt-6 py-4 text-sm text-muted">
          登录后才能发表帖子，请使用页面右上角的登录入口。
        </div>

        <XAsyncContent
          v-else
          :loading="!categoriesReady && categoryStore.loading"
          :error="categoriesReady ? undefined : categoryStore.error"
          error-title="分类加载失败"
          @retry="categoryStore.refresh"
        >
          <template #loading>
            <p class="mt-6 py-4 text-sm text-muted">正在加载分类…</p>
          </template>
          <PostForm
            v-model:title="title"
            v-model:category="categorySlug"
            v-model:content="content"
            v-model:tag-ids="selectedTagIds"
            class="mt-6"
            :categories="categoryStore.writableCategories"
            :tags="tags"
            :submitting="submitting"
            submit-label="发布帖子"
            submitting-label="发布中…"
            title-placeholder="用一句话概括你想讨论的内容"
            content-placeholder="详细说明你想分享或讨论的内容…"
            @category-change="changeCategory"
            @submit="submitPost"
          >
            <template #notice>
              <PostPublishingNotice />
            </template>
          </PostForm>
        </XAsyncContent>
      </section>
    </div>
  </div>
</template>
