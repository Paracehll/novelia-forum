<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { RouterLink, useRoute, useRouter } from 'vue-router';

import { updatePost, type Post } from '@/api';
import { useWebKit, XButton } from '@novelia/web-kit';
import { XAsyncContent } from '@novelia/web-kit';
import { useUnsavedChangesGuard } from '@/composables/useUnsavedChangesGuard';
import { getApiErrorMessage, Notify } from '@novelia/web-kit';
import { useCategoryStore } from '@/stores/category';
import { usePostQuery, usePostStore } from '@/stores/post';

import PostForm from './PostForm.vue';

const { whoami } = useWebKit();
const route = useRoute();
const router = useRouter();
const categoryStore = useCategoryStore();
const postStore = usePostStore();

const title = ref('');
const content = ref('');
const categorySlug = ref('');
const selectedTagIds = ref<number[]>([]);
const submitting = ref(false);
const savedSnapshot = ref('');

const postId = computed(() => {
  const value = Number(route.params.id);
  return Number.isSafeInteger(value) && value > 0 ? value : 0;
});
const categoriesReady = ref(false);
const {
  post,
  loading: postLoading,
  error,
  retry,
} = usePostQuery(postId, { editing: true, enabled: categoriesReady });
const loading = computed(() => !categoriesReady.value || postLoading.value);
const canEdit = computed(
  () =>
    (post.value != null &&
      categoryStore.canPublish(
        categoryStore.categories.find(
          (item) => item.id === post.value?.categoryId,
        )?.slug ?? '',
      ) &&
      whoami.value.user?.id === post.value.authorId) ||
    whoami.value.asAdmin,
);
const selectedCategory = computed(
  () =>
    categoryStore.categories.find(
      (category) => category.slug === categorySlug.value,
    ) ?? categoryStore.defaultCategory,
);
const tags = computed(() => selectedCategory.value.tags);
const detailRoute = computed(() => ({
  name: 'post-detail' as const,
  params: { id: postId.value },
}));
const hasUnsavedChanges = computed(
  () => savedSnapshot.value !== '' && savedSnapshot.value !== formSnapshot(),
);

useUnsavedChangesGuard(hasUnsavedChanges);

function formSnapshot() {
  return JSON.stringify([
    title.value,
    content.value,
    categorySlug.value,
    [...selectedTagIds.value].sort((left, right) => left - right),
  ]);
}

function applyPost(value: Post) {
  title.value = value.title;
  content.value = value.content;
  const category =
    categoryStore.categories.find((item) => item.id === value.categoryId) ??
    categoryStore.defaultCategory;
  categorySlug.value = category.slug;
  const validTagIds = new Set(category.tags.map((tag) => tag.id));
  selectedTagIds.value = value.tags
    .map((tag) => tag.id)
    .filter((id) => validTagIds.has(id));
  savedSnapshot.value = formSnapshot();
}

async function loadPost() {
  await categoryStore.initialize();
  await retry();
}

function changeCategory() {
  selectedTagIds.value = [];
}

async function save() {
  if (!post.value || submitting.value) return;
  submitting.value = true;
  try {
    const updatedPost = await updatePost(post.value.id, {
      categoryId: selectedCategory.value.id,
      title: title.value.trim(),
      content: content.value,
      tagIds: selectedTagIds.value,
    });
    applyPost(updatedPost);
    postStore.setPost(updatedPost);
    Notify.success('帖子修改已保存');
    await router.replace({
      name: 'post-detail',
      params: { id: updatedPost.id },
    });
  } catch (reason) {
    Notify.error(await getApiErrorMessage(reason, '更新帖子失败'));
  } finally {
    submitting.value = false;
  }
}

function cancel() {
  void router.push(detailRoute.value);
}

watch(
  [post, categoriesReady],
  ([value, ready]) => {
    if (!ready || !value || hasUnsavedChanges.value) return;
    applyPost(value);
  },
  { immediate: true },
);
watch(
  postId,
  () => {
    savedSnapshot.value = '';
  },
  { flush: 'sync' },
);
void categoryStore.initialize().then(() => {
  categoriesReady.value = true;
});
</script>

<template>
  <div class="page-container py-4 md:py-6">
    <div class="mx-auto max-w-4xl">
      <XAsyncContent
        :loading="loading"
        :error="error"
        size="large"
        heading-tag="h1"
        error-title="帖子加载失败"
        @retry="loadPost"
      >
        <template #loading>
          <div class="py-6">
            <div class="h-7 w-32 animate-pulse rounded-sm bg-border" />
            <div class="mt-6 h-10 w-full animate-pulse rounded-sm bg-divider" />
            <div class="mt-5 h-10 w-full animate-pulse rounded-sm bg-divider" />
            <div class="mt-5 h-80 w-full animate-pulse rounded-sm bg-divider" />
          </div>
        </template>

        <section v-if="post && canEdit">
          <h1 class="text-xl font-bold text-ink">编辑帖子</h1>
          <PostForm
            v-model:title="title"
            v-model:category="categorySlug"
            v-model:content="content"
            v-model:tag-ids="selectedTagIds"
            class="mt-6"
            :categories="categoryStore.writableCategories"
            :tags="tags"
            :submitting="submitting"
            submit-label="保存修改"
            submitting-label="保存中…"
            title-placeholder="用一句话概括你想讨论的内容"
            content-placeholder="详细说明你想分享或讨论的内容…"
            show-cancel
            @category-change="changeCategory"
            @submit="save"
            @cancel="cancel"
          />
        </section>

        <section v-else-if="post" class="py-8 text-center">
          <h1 class="text-xl font-bold text-ink">无法编辑帖子</h1>
          <p class="mt-2 text-sm text-muted">
            {{
              whoami.isSignedIn
                ? '只有具有该板块发帖权限的作者或管理员可以编辑这篇帖子。'
                : '登录后才能编辑帖子，请使用页面右上角的登录入口。'
            }}
          </p>
          <XButton
            :as="RouterLink"
            :to="detailRoute"
            variant="outline"
            class="mt-5"
          >
            返回帖子详情
          </XButton>
        </section>
      </XAsyncContent>
    </div>
  </div>
</template>
