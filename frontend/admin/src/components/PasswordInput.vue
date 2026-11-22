<script setup lang="ts">
import { ref, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { Eye, EyeOff } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  modelValue: string
  inputClass?: string
}>(), {
  inputClass: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const { t } = useI18n()
const attrs = useAttrs()
const visible = ref(false)
</script>

<template>
  <div class="relative">
    <Input
      v-bind="attrs"
      :model-value="props.modelValue"
      :type="visible ? 'text' : 'password'"
      :class="['pr-10', props.inputClass]"
      @update:model-value="(value) => emit('update:modelValue', String(value))"
    />
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      class="absolute right-1 top-1/2 -translate-y-1/2"
      :aria-label="t(visible ? 'admin.settings.security.hidePassword' : 'admin.settings.security.showPassword')"
      :title="t(visible ? 'admin.settings.security.hidePassword' : 'admin.settings.security.showPassword')"
      @click="visible = !visible"
    >
      <Eye v-if="visible" class="h-4 w-4" aria-hidden="true" />
      <EyeOff v-else class="h-4 w-4" aria-hidden="true" />
    </Button>
  </div>
</template>
