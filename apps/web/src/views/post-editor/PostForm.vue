<script setup lang="ts">
import { computed, useId } from 'vue';

import type { PostTag } from '@/api';
import { XButton } from '@novelia/web-kit';
import { XSelect } from '@novelia/web-kit';
import MarkdownEditor from '@/components/markdown/MarkdownEditor.vue';
import MarkdownHelpDialog from '@/components/markdown/MarkdownHelpDialog.vue';

const props = withDefaults(
  defineProps<{
    categories: { id: number; slug: string; title: string }[];
    tags: PostTag[];
    submitting: boolean;
    submitLabel: string;
    submittingLabel: string;
    titlePlaceholder?: string;
    contentPlaceholder?: string;
    showCancel?: boolean;
  }>(),
  {
    titlePlaceholder: '',
    contentPlaceholder: '使用 Markdown 输入内容…',
    showCancel: false,
  },
);

const emit = defineEmits<{
  submit: [];
  cancel: [];
  categoryChange: [];
}>();
const title = defineModel<string>('title', { required: true });
const category = defineModel<string>('category', { required: true });
const content = defineModel<string>('content', { required: true });
const tagIds = defineModel<number[]>('tagIds', { required: true });
const categoryOptions = computed(() =>
  props.categories.map((categoryOption) => ({
    label: categoryOption.title,
    value: categoryOption.slug,
  })),
);
const contentHintId = useId();
const titleLength = computed(() => Array.from(title.value.trim()).length);
const contentLength = computed(() => Array.from(content.value).length);
const maxTags = 3;
const titleHint = computed(() => {
  if (titleLength.value < 2) return '标题至少需要 2 字';
  if (titleLength.value > 100) return '标题不能超过 100 字';
  return '';
});
const contentHint = computed(() => {
  if (!content.value.trim()) return '请输入正文';
  if (contentLength.value > 20000) return '正文不能超过 20000 字';
  return '';
});
const canSubmit = computed(
  () =>
    titleLength.value >= 2 &&
    titleLength.value <= 100 &&
    Boolean(content.value.trim()) &&
    contentLength.value <= 20000 &&
    tagIds.value.length <= maxTags &&
    !props.submitting,
);

function submit() {
  if (canSubmit.value) emit('submit');
}
</script>

<template>
  <form class="space-y-5" @submit.prevent="submit">
    <slot name="notice" />

    <div>
      <label for="post-title" class="mb-2 block text-sm font-semibold text-ink">
        标题
      </label>
      <input
        id="post-title"
        v-model="title"
        type="text"
        class="block min-h-10 w-full rounded-md border border-border bg-transparent px-3 text-sm text-ink outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/15 disabled:cursor-not-allowed disabled:opacity-50"
        :placeholder="titlePlaceholder"
        :disabled="submitting"
        required
        :aria-describedby="titleHint ? 'post-title-hint' : undefined"
        :aria-invalid="
          titleLength > 100 || (titleLength > 0 && titleLength < 2) || undefined
        "
      />
      <div class="mt-1 flex items-center justify-between gap-3 text-xs">
        <p
          v-if="titleHint"
          id="post-title-hint"
          :class="title ? 'text-warning' : 'text-muted'"
          aria-live="polite"
        >
          {{ titleHint }}
        </p>
        <p class="ml-auto text-muted">{{ titleLength }} / 100</p>
      </div>
    </div>

    <div>
      <label
        for="post-category"
        class="mb-2 block text-sm font-semibold text-ink"
      >
        分类
      </label>
      <XSelect
        id="post-category"
        v-model="category"
        :options="categoryOptions"
        :disabled="submitting"
        rounded
        required
        @change="emit('categoryChange')"
      />
    </div>

    <fieldset :disabled="submitting">
      <legend class="mb-2 text-sm font-semibold text-ink">
        标签（最多 3 个）
      </legend>
      <div v-if="tags.length" class="flex flex-wrap gap-2">
        <label
          v-for="tag in tags"
          :key="tag.id"
          class="cursor-pointer rounded-sm border px-3 py-1.5 text-xs font-medium transition-colors"
          :class="
            tagIds.includes(tag.id)
              ? 'border-primary bg-primary-soft text-primary'
              : tagIds.length >= maxTags
                ? 'cursor-not-allowed border-border text-muted opacity-50'
                : 'border-border text-muted hover:border-primary hover:text-primary'
          "
        >
          <input
            v-model="tagIds"
            type="checkbox"
            class="sr-only"
            :value="tag.id"
            :disabled="tagIds.length >= maxTags && !tagIds.includes(tag.id)"
          />
          {{ tag.name }}
        </label>
      </div>
      <p v-else class="text-sm text-muted">这个分类暂时没有可用标签。</p>
    </fieldset>

    <div>
      <div class="mb-2 flex items-center justify-between gap-3">
        <label class="text-sm font-semibold text-ink">正文</label>
        <MarkdownHelpDialog mode="article" />
      </div>
      <MarkdownEditor
        v-model="content"
        mode="article"
        :placeholder="contentPlaceholder"
        :rows="14"
        :disabled="submitting"
        :described-by="contentHint ? contentHintId : undefined"
        :invalid="contentLength > 20000"
      />
      <div class="mt-1 flex items-center justify-between gap-3 text-xs">
        <p
          v-if="contentHint"
          :id="contentHintId"
          :class="content ? 'text-warning' : 'text-muted'"
          aria-live="polite"
        >
          {{ contentHint }}
        </p>
        <p class="ml-auto text-muted">{{ contentLength }} / 20000</p>
      </div>
    </div>

    <div
      class="flex flex-wrap items-center justify-end gap-3 border-t border-divider pt-5"
    >
      <div class="flex items-center gap-3">
        <XButton
          v-if="showCancel"
          variant="outline"
          :disabled="submitting"
          @click="emit('cancel')"
        >
          取消
        </XButton>
        <XButton type="submit" :disabled="!canSubmit">
          {{ submitting ? submittingLabel : submitLabel }}
        </XButton>
      </div>
    </div>
  </form>
</template>
