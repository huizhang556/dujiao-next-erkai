<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, CheckCircle2, Copy, ExternalLink, RefreshCw } from 'lucide-vue-next'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { adminAPI } from '@/api/admin'
import { notifyError } from '@/utils/notify'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()
const { t } = useI18n()

interface UpdateCheckResult {
  current_version: string
  latest_version: string
  has_update: boolean
  release_url?: string
  published_at?: string | null
}

const dockerUpgradeCommand = [
  'docker compose --env-file .env -f docker-compose.postgres.yml down',
  'docker compose --env-file .env -f docker-compose.postgres.yml pull',
  'docker compose --env-file .env -f docker-compose.postgres.yml up -d',
].join('\n')

const checkLoading = ref(false)
const checkResult = ref<UpdateCheckResult | null>(null)
const checkError = ref('')
const copied = ref(false)

function extractErrorMessage(err: any): string {
  return err?.response?.data?.msg || err?.message || ''
}

function formatPublishedAt(raw: string | null | undefined) {
  if (!raw) return ''
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? raw : date.toLocaleString()
}

async function loadCheck() {
  checkLoading.value = true
  checkError.value = ''
  checkResult.value = null
  try {
    const res = await adminAPI.checkSystemUpdate()
    const payload = res.data?.data as UpdateCheckResult | undefined
    checkResult.value = payload || null
    if (!payload) checkError.value = t('admin.updateCheck.failedGeneric')
  } catch (err: any) {
    const status = err?.response?.status
    const code = err?.response?.data?.status_code
    const message = extractErrorMessage(err)
    if (status === 429 || code === 429) {
      checkError.value = t('admin.updateCheck.rateLimited')
    } else {
      checkError.value = message ? t('admin.updateCheck.failed', { message }) : t('admin.updateCheck.failedGeneric')
    }
  } finally {
    checkLoading.value = false
  }
}

async function copyDockerCommand() {
  try {
    await navigator.clipboard.writeText(dockerUpgradeCommand)
    copied.value = true
    window.setTimeout(() => { copied.value = false }, 2000)
  } catch {
    notifyError(t('admin.systemUpdate.copyFailed'))
  }
}

watch(() => props.open, (open) => {
  if (open) {
    copied.value = false
    void loadCheck()
  }
})
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-h-[90vh] overflow-y-auto sm:max-w-lg">
      <DialogHeader><DialogTitle>{{ t('admin.updateCheck.title') }}</DialogTitle></DialogHeader>
      <div class="space-y-3 text-sm">
        <div v-if="checkLoading" class="flex items-center gap-2 py-4 text-muted-foreground">
          <RefreshCw class="h-4 w-4 animate-spin" /><span>{{ t('admin.updateCheck.checking') }}</span>
        </div>
        <div v-else-if="checkError" class="flex items-start gap-2 text-destructive">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" /><span>{{ checkError }}</span>
        </div>
        <template v-else-if="checkResult">
          <div class="flex items-start gap-2 rounded-md border px-3 py-2" :class="checkResult.has_update ? 'border-amber-500/40 bg-amber-500/5 text-amber-600 dark:text-amber-400' : 'border-emerald-500/40 bg-emerald-500/5 text-emerald-600 dark:text-emerald-400'">
            <component :is="checkResult.has_update ? AlertCircle : CheckCircle2" class="mt-0.5 h-4 w-4 shrink-0" />
            <span>{{ checkResult.has_update ? t('admin.updateCheck.hasUpdate', { version: checkResult.latest_version }) : t('admin.updateCheck.latest', { version: checkResult.current_version }) }}</span>
          </div>
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-muted-foreground">
            <dt>{{ t('admin.updateCheck.currentLabel') }}</dt><dd class="text-foreground">{{ checkResult.current_version }}</dd>
            <dt>{{ t('admin.updateCheck.latestLabel') }}</dt><dd class="text-foreground">{{ checkResult.latest_version }}</dd>
            <template v-if="checkResult.published_at"><dt>{{ t('admin.updateCheck.publishedLabel') }}</dt><dd class="text-foreground">{{ formatPublishedAt(checkResult.published_at) }}</dd></template>
          </dl>
          <div v-if="checkResult.has_update" class="space-y-2 rounded-md border bg-muted/40 px-3 py-2.5">
            <p class="text-xs text-muted-foreground">{{ t('admin.systemUpdate.dockerHint') }}</p>
            <div class="flex items-start gap-2">
              <pre class="min-w-0 flex-1 overflow-x-auto whitespace-pre-wrap break-words rounded bg-background px-2 py-1.5 text-xs">{{ dockerUpgradeCommand }}</pre>
              <Button size="icon-sm" variant="outline" :title="t('admin.systemUpdate.copy')" @click="copyDockerCommand"><CheckCircle2 v-if="copied" class="h-3.5 w-3.5 text-emerald-500" /><Copy v-else class="h-3.5 w-3.5" /></Button>
            </div>
          </div>
        </template>
      </div>
      <DialogFooter class="gap-2 sm:gap-2">
        <a v-if="checkResult?.release_url" :href="checkResult.release_url" target="_blank" rel="noopener noreferrer" class="inline-flex h-9 items-center justify-center gap-1.5 rounded-md border px-3 text-sm font-medium hover:bg-accent"><ExternalLink class="h-4 w-4" />{{ t('admin.updateCheck.viewRelease') }}</a>
        <Button variant="outline" size="sm" @click="emit('update:open', false)">{{ t('admin.updateCheck.close') }}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
