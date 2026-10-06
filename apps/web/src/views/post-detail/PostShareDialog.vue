<script setup lang="ts">
import { CloseOutlined } from '@vicons/material';
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui';
import { computed, nextTick, ref, watch } from 'vue';

import { Notify, XButton } from '@novelia/web-kit';
import { copyText, postShareMarkdown, postShareUrl } from '@/utils/postShare';

const props = defineProps<{
  open: boolean;
  post: { id: number; title: string };
}>();

const emit = defineEmits<{ 'update:open': [open: boolean] }>();

const linkInput = ref<HTMLInputElement>();
const copying = ref(false);

const link = computed(() => postShareUrl(props.post));
const markdown = computed(() => postShareMarkdown(props.post));

// 打开后选中链接，用户也可以直接用系统快捷键复制。
watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    copying.value = false;
    await nextTick();
    linkInput.value?.select();
  },
);

function selectLink() {
  linkInput.value?.select();
}

async function copy(content: string, successMessage: string) {
  if (copying.value) return;
  copying.value = true;
  try {
    await copyText(content);
    Notify.success(successMessage);
    emit('update:open', false);
  } catch {
    Notify.error('复制失败，请手动选中链接复制');
  } finally {
    copying.value = false;
  }
}

function copyLink() {
  void copy(link.value, '帖子链接已复制');
}

function copyMarkdown() {
  void copy(markdown.value, 'Markdown 引用已复制');
}
</script>

<template>
  <DialogRoot :open="open" @update:open="emit('update:open', $event)">
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-40 bg-black/45" />
      <DialogContent
        class="fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-md border border-border bg-surface p-5 shadow-2xl outline-none sm:p-6"
      >
        <DialogTitle class="pr-10 text-lg font-semibold text-ink">
          分享帖子
        </DialogTitle>
        <DialogDescription class="mt-2 pr-10 text-sm leading-6 text-muted">
          复制链接发送给好友，或复制 Markdown 引用到其他平台。
        </DialogDescription>
        <DialogClose as-child>
          <XButton
            class="absolute top-4 right-4"
            variant="subtle"
            size="icon-sm"
            aria-label="关闭"
          >
            <CloseOutlined class="size-5" aria-hidden="true" />
          </XButton>
        </DialogClose>

        <label
          class="mt-5 block text-xs font-medium text-muted"
          for="post-share-link"
        >
          帖子链接
        </label>
        <input
          id="post-share-link"
          ref="linkInput"
          :value="link"
          readonly
          class="mt-2 min-h-10 w-full rounded-sm border border-border bg-transparent px-3 text-sm text-ink outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/15"
          @focus="selectLink"
        />

        <div class="mt-5 flex flex-wrap justify-end gap-3">
          <XButton variant="outline" :disabled="copying" @click="copyMarkdown">
            复制 Markdown
          </XButton>
          <XButton :disabled="copying" @click="copyLink">复制链接</XButton>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
