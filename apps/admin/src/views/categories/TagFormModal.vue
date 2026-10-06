<script setup lang="ts">
import {
  NButton,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
} from 'naive-ui';

defineProps<{
  show: boolean;
  editing: boolean;
  saving: boolean;
}>();

const emit = defineEmits<{
  close: [];
  save: [];
}>();

const name = defineModel<string>('name', { required: true });
const color = defineModel<number>('color', { required: true });
const sortOrder = defineModel<number>('sortOrder', { required: true });
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    :title="editing ? '编辑标签' : '新建标签'"
    class="form-modal"
    :mask-closable="!saving"
    :close-on-esc="!saving"
    @update:show="(value) => !value && emit('close')"
  >
    <n-form label-placement="top">
      <n-form-item label="标签名称" required>
        <n-input v-model:value="name" placeholder="输入标签名称" />
      </n-form-item>
      <div class="form-grid">
        <n-form-item label="预定义色号">
          <n-input-number v-model:value="color" :min="0" />
        </n-form-item>
        <n-form-item label="排序值">
          <n-input-number v-model:value="sortOrder" />
        </n-form-item>
      </div>
    </n-form>
    <template #footer>
      <div class="modal-actions">
        <n-button :disabled="saving" @click="emit('close')">取消</n-button>
        <n-button
          type="primary"
          :loading="saving"
          :disabled="!name.trim()"
          @click="emit('save')"
        >
          保存
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
:global(.form-modal) {
  width: min(520px, calc(100vw - 32px));
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 16px;
}
</style>
