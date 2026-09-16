<script lang="ts" setup>
import {computed, onMounted, onUnmounted, ref, watch} from 'vue'
import {
  ChevronDownIcon,
  FolderIcon,
  FolderOpenIcon,
  ImageIcon,
  InfoIcon,
  LoaderCircleIcon,
  Settings2Icon,
  SparklesIcon,
  SquareIcon,
  UploadIcon,
  XIcon,
} from '@lucide/vue'
import logoUrl from '@/assets/images/logo-universal.png'
import ImageCompare from '@/components/ImageCompare.vue'
import {Badge} from '@/components/ui/badge'
import {Button} from '@/components/ui/button'
import {Input} from '@/components/ui/input'
import {Label} from '@/components/ui/label'
import {NativeSelect, NativeSelectOption} from '@/components/ui/native-select'
import {Progress} from '@/components/ui/progress'
import {Switch} from '@/components/ui/switch'
import {
  CancelUpscale,
  DefaultPaths,
  ListComparePairs,
  ListModels,
  OpenInExplorer,
  PathKind,
  ReadImageDataURL,
  ResolveDroppedPaths,
  RunUpscale,
  SelectBin,
  SelectInputDirectory,
  SelectInputFile,
  SelectModelsDirectory,
  SelectOutputDirectory,
  SelectOutputFile,
  SuggestOutput,
} from '../wailsjs/go/main/App'
import {EventsOn, OnFileDrop, OnFileDropOff} from '../wailsjs/runtime/runtime'
import type {main} from '../wailsjs/go/models'

const binPath = ref('')
const modelsPath = ref('')
const inputPath = ref('')
const outputPath = ref('')
const modelName = ref('upscayl-standard-4x')
const scale = ref('4')
const format = ref('png')
const tileSize = ref('0')
const gpuId = ref('auto')
const compress = ref('0')
const tta = ref(false)
const verbose = ref(false)

const models = ref<main.ModelInfo[]>([])
const running = ref(false)
const progress = ref(0)
const status = ref<'idle' | 'running' | 'done' | 'error' | 'cancelled'>('idle')
const logs = ref<string[]>([])
const errorText = ref('')
const dragOver = ref(false)
const sidePanel = ref<'none' | 'advanced' | 'logs'>('none')
const showAbout = ref(false)

const showAdvanced = computed(() => sidePanel.value === 'advanced')
const showLogs = computed(() => sidePanel.value === 'logs')

function toggleAdvanced() {
  sidePanel.value = sidePanel.value === 'advanced' ? 'none' : 'advanced'
}

function toggleLogs() {
  sidePanel.value = sidePanel.value === 'logs' ? 'none' : 'logs'
}

const comparePairs = ref<main.ComparePair[]>([])
const selectedPair = ref('')
const beforeSrc = ref('')
const afterSrc = ref('')
const compareLoading = ref(false)
const compareError = ref('')

const statusLabel = computed(() => {
  switch (status.value) {
    case 'running':
      return '处理中'
    case 'done':
      return '已完成'
    case 'error':
      return '失败'
    case 'cancelled':
      return '已取消'
    default:
      return '就绪'
  }
})

const statusVariant = computed(() => {
  switch (status.value) {
    case 'running':
      return 'secondary'
    case 'done':
      return 'default'
    case 'error':
      return 'destructive'
    default:
      return 'outline'
  }
})

const canCompare = computed(() => Boolean(beforeSrc.value && afterSrc.value))

const inputLabel = computed(() => shortPath(inputPath.value) || '拖入图片或文件夹')
const outputLabel = computed(() => shortPath(outputPath.value) || '自动建议输出路径')

function shortPath(path: string) {
  if (!path) {
    return ''
  }
  const parts = path.split(/[\\/]/).filter(Boolean)
  if (parts.length <= 2) {
    return path
  }
  return `…/${parts.slice(-2).join('/')}`
}

function pushLog(line: string) {
  logs.value.push(line)
  if (logs.value.length > 800) {
    logs.value = logs.value.slice(-600)
  }
}

async function refreshModels() {
  try {
    const list = await ListModels(modelsPath.value)
    models.value = list ?? []
    if (models.value.length && !models.value.some((m) => m.name === modelName.value)) {
      modelName.value = models.value[0].name
    }
  } catch (err) {
    models.value = []
    pushLog(`读取模型失败: ${String(err)}`)
  }
}

async function applySuggestedOutput() {
  if (!inputPath.value) {
    return
  }
  const suggested = await SuggestOutput(inputPath.value, format.value)
  if (suggested) {
    outputPath.value = suggested
  }
}

async function setInputPath(path: string) {
  inputPath.value = path
  await applySuggestedOutput()
  await refreshCompare()
}

async function pick(getter: () => Promise<string>, assign: (v: string) => void | Promise<void>) {
  const path = await getter()
  if (path) {
    await assign(path)
  }
}

async function onPickInputFile() {
  await pick(SelectInputFile, setInputPath)
}

async function onPickInputDir() {
  await pick(SelectInputDirectory, setInputPath)
}

async function onPickOutput() {
  const kind = inputPath.value ? await PathKind(inputPath.value) : 'file'
  if (kind === 'dir') {
    await pick(SelectOutputDirectory, async (path) => {
      outputPath.value = path
      await refreshCompare()
    })
    return
  }
  const name = outputPath.value.split(/[\\/]/).pop() || 'upscaled.png'
  await pick(() => SelectOutputFile(name), async (path) => {
    outputPath.value = path
    await refreshCompare()
  })
}

async function handleDroppedPaths(paths: string[]) {
  if (running.value || !paths?.length) {
    return
  }
  dragOver.value = false
  try {
    const resolved = await ResolveDroppedPaths(paths)
    if (resolved) {
      await setInputPath(resolved)
      pushLog(`已拖入: ${resolved}`)
    }
  } catch (err) {
    errorText.value = String(err)
    pushLog(String(err))
  }
}

async function loadPairPreview(pair: main.ComparePair | undefined) {
  beforeSrc.value = ''
  afterSrc.value = ''
  compareError.value = ''
  if (!pair) {
    return
  }
  compareLoading.value = true
  try {
    const [before, after] = await Promise.all([
      ReadImageDataURL(pair.before),
      ReadImageDataURL(pair.after),
    ])
    beforeSrc.value = before
    afterSrc.value = after
  } catch (err) {
    compareError.value = String(err)
  } finally {
    compareLoading.value = false
  }
}

async function refreshCompare() {
  comparePairs.value = []
  selectedPair.value = ''
  beforeSrc.value = ''
  afterSrc.value = ''
  compareError.value = ''
  if (!inputPath.value || !outputPath.value) {
    return
  }
  try {
    const pairs = await ListComparePairs(inputPath.value, outputPath.value)
    comparePairs.value = pairs ?? []
    if (comparePairs.value.length) {
      selectedPair.value = comparePairs.value[0].before
      await loadPairPreview(comparePairs.value[0])
    }
  } catch (err) {
    compareError.value = String(err)
  }
}

watch(selectedPair, async (value) => {
  const pair = comparePairs.value.find((item) => item.before === value)
  await loadPairPreview(pair)
})

watch(format, async () => {
  if (inputPath.value && !running.value) {
    await applySuggestedOutput()
  }
})

async function start() {
  if (running.value) {
    return
  }
  errorText.value = ''
  logs.value = []
  progress.value = 0
  running.value = true
  status.value = 'running'
  beforeSrc.value = ''
  afterSrc.value = ''
  try {
    const result = await RunUpscale({
      binPath: binPath.value,
      modelsPath: modelsPath.value,
      inputPath: inputPath.value,
      outputPath: outputPath.value,
      modelName: modelName.value,
      scale: Number(scale.value) || 4,
      format: format.value,
      tileSize: Number(tileSize.value) || 0,
      gpuId: gpuId.value,
      compress: Number(compress.value) || 0,
      tta: tta.value,
      verbose: verbose.value,
    })
    if (result.ok) {
      status.value = 'done'
      await refreshCompare()
    } else if (status.value === 'running') {
      status.value = result.error === '已取消' ? 'cancelled' : 'error'
      errorText.value = result.error
      sidePanel.value = 'logs'
    } else if (status.value === 'done') {
      await refreshCompare()
    }
  } catch (err) {
    status.value = 'error'
    errorText.value = String(err)
    pushLog(String(err))
    sidePanel.value = 'logs'
  } finally {
    running.value = false
  }
}

async function cancel() {
  await CancelUpscale()
}

const unsubs: Array<() => void> = []

onMounted(async () => {
  unsubs.push(EventsOn('upscale:log', (line: string) => pushLog(line)))
  unsubs.push(EventsOn('upscale:progress', (value: number) => {
    progress.value = Math.max(0, Math.min(100, Number(value) || 0))
  }))
  unsubs.push(EventsOn('upscale:status', (next: string) => {
    if (next === 'running' || next === 'done' || next === 'error' || next === 'cancelled') {
      status.value = next
    }
  }))

  OnFileDrop((_x, _y, paths) => {
    void handleDroppedPaths(paths)
  }, true)

  const paths = await DefaultPaths()
  binPath.value = paths.binPath
  modelsPath.value = paths.modelsPath
  await refreshModels()
})

onUnmounted(() => {
  unsubs.forEach((fn) => fn())
  OnFileDropOff()
})
</script>

<template>
  <div class="bg-background text-foreground flex h-full overflow-hidden">
    <!-- 左侧控制栏 -->
    <aside class="border-border flex w-[340px] shrink-0 flex-col border-r">
      <!-- 固定区：开始超分及以上 -->
      <div class="flex shrink-0 flex-col gap-4 p-4">
        <!-- 拖放 / 输入 -->
        <section class="space-y-2">
          <div
            class="flex min-h-[120px] flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-5 text-center transition-colors"
            :class="dragOver ? 'border-primary bg-primary/10' : 'border-border bg-muted/30'"
            style="--wails-drop-target: drop"
            @dragenter.prevent="dragOver = true"
            @dragover.prevent="dragOver = true"
            @dragleave.prevent="dragOver = false"
            @drop.prevent="dragOver = false"
          >
            <UploadIcon class="text-muted-foreground size-6" />
            <p class="text-sm font-medium">{{ inputPath ? '已选择输入' : '拖入图片 / 文件夹' }}</p>
            <p class="text-muted-foreground max-w-full truncate px-1 font-mono text-[11px]" :title="inputPath">
              {{ inputLabel }}
            </p>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <Button variant="outline" size="sm" :disabled="running" @click="onPickInputFile">
              <ImageIcon />
              选图片
            </Button>
            <Button variant="outline" size="sm" :disabled="running" @click="onPickInputDir">
              <FolderIcon />
              选文件夹
            </Button>
          </div>
        </section>

        <!-- 核心参数：标签与选项同一行 -->
        <section class="grid grid-cols-2 gap-3">
          <div class="flex items-center gap-2">
            <Label class="shrink-0 text-xs">倍率</Label>
            <NativeSelect v-model="scale" class="min-w-0 flex-1" :disabled="running">
              <NativeSelectOption value="2">2x</NativeSelectOption>
              <NativeSelectOption value="3">3x</NativeSelectOption>
              <NativeSelectOption value="4">4x</NativeSelectOption>
            </NativeSelect>
          </div>
          <div class="flex items-center gap-2">
            <Label class="shrink-0 text-xs">格式</Label>
            <NativeSelect v-model="format" class="min-w-0 flex-1" :disabled="running">
              <NativeSelectOption value="png">PNG</NativeSelectOption>
              <NativeSelectOption value="jpg">JPG</NativeSelectOption>
              <NativeSelectOption value="webp">WebP</NativeSelectOption>
            </NativeSelect>
          </div>
        </section>

        <!-- 操作 -->
        <section class="space-y-2">
          <div class="flex gap-2">
            <Button class="flex-1" :disabled="running || !inputPath || !outputPath" @click="start">
              <LoaderCircleIcon v-if="running" class="animate-spin" />
              <SparklesIcon v-else />
              {{ running ? '处理中…' : '开始超分' }}
            </Button>
            <Button variant="destructive" size="icon" :disabled="!running" @click="cancel" title="取消">
              <SquareIcon />
            </Button>
          </div>
          <div v-if="running || progress > 0" class="space-y-1.5">
            <div class="text-muted-foreground flex justify-between text-xs">
              <span>进度</span>
              <span>{{ progress.toFixed(0) }}%</span>
            </div>
            <Progress :model-value="progress" />
          </div>
          <p v-if="errorText" class="text-destructive text-xs leading-relaxed">{{ errorText }}</p>
        </section>
      </div>

      <!-- 高级 / 日志：标题固定，仅内容区内滚动 -->
      <div class="border-border flex min-h-0 flex-1 flex-col border-t">
        <section
          class="flex min-h-0 flex-col"
          :class="showAdvanced ? 'min-h-0 flex-1' : 'shrink-0'"
        >
          <button
            type="button"
            class="text-muted-foreground hover:text-foreground flex w-full shrink-0 items-center justify-between px-4 py-3 text-xs"
            @click="toggleAdvanced"
          >
            <span class="inline-flex items-center gap-1.5">
              <Settings2Icon class="size-3.5" />
              高级选项
            </span>
            <ChevronDownIcon class="size-3.5 transition-transform" :class="showAdvanced && 'rotate-180'" />
          </button>
          <div v-if="showAdvanced" class="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 pb-3">
            <div class="space-y-1.5">
              <div class="flex items-center justify-between">
                <Label class="text-xs">输出</Label>
                <button
                  type="button"
                  class="text-muted-foreground hover:text-foreground text-xs underline-offset-2 hover:underline disabled:opacity-40"
                  :disabled="!outputPath"
                  @click="OpenInExplorer(outputPath)"
                >
                  打开位置
                </button>
              </div>
              <div class="flex gap-2">
                <Input
                  v-model="outputPath"
                  class="h-8 font-mono text-[11px]"
                  :title="outputPath"
                  :placeholder="outputLabel"
                />
                <Button variant="outline" size="sm" class="shrink-0" :disabled="running" @click="onPickOutput">
                  <FolderOpenIcon />
                </Button>
              </div>
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">模型</Label>
              <NativeSelect v-model="modelName" class="w-full" :disabled="running">
                <NativeSelectOption v-for="item in models" :key="item.name" :value="item.name">
                  {{ item.label }}
                </NativeSelectOption>
              </NativeSelect>
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">瓦片大小</Label>
              <Input v-model="tileSize" class="h-8" :disabled="running" inputmode="numeric" />
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">GPU</Label>
              <Input v-model="gpuId" class="h-8" :disabled="running" placeholder="auto" />
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">压缩</Label>
              <Input v-model="compress" class="h-8" :disabled="running" inputmode="numeric" />
            </div>
            <div class="flex items-center justify-between">
              <Label for="tta" class="text-xs">TTA</Label>
              <Switch id="tta" v-model="tta" :disabled="running" />
            </div>
            <div class="flex items-center justify-between">
              <Label for="verbose" class="text-xs">详细日志</Label>
              <Switch id="verbose" v-model="verbose" :disabled="running" />
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">upscayl-bin</Label>
              <div class="flex gap-2">
                <Input v-model="binPath" class="h-8 font-mono text-[11px]" :disabled="running" />
                <Button variant="outline" size="sm" class="shrink-0" :disabled="running" @click="pick(SelectBin, (v) => { binPath = v })">
                  …
                </Button>
              </div>
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs">模型目录</Label>
              <div class="flex gap-2">
                <Input v-model="modelsPath" class="h-8 font-mono text-[11px]" :disabled="running" />
                <Button
                  variant="outline"
                  size="sm"
                  class="shrink-0"
                  :disabled="running"
                  @click="pick(SelectModelsDirectory, async (v) => { modelsPath = v; await refreshModels() })"
                >
                  …
                </Button>
              </div>
            </div>
          </div>
        </section>

        <section
          class="border-border flex min-h-0 flex-col border-t"
          :class="showLogs ? 'min-h-0 flex-1' : 'shrink-0'"
        >
          <button
            type="button"
            class="text-muted-foreground hover:text-foreground flex w-full shrink-0 items-center justify-between px-4 py-3 text-xs"
            @click="toggleLogs"
          >
            <span>运行日志{{ logs.length ? `（${logs.length}）` : '' }}</span>
            <ChevronDownIcon class="size-3.5 transition-transform" :class="showLogs && 'rotate-180'" />
          </button>
          <pre
            v-if="showLogs"
            class="bg-muted/40 text-muted-foreground mx-4 mb-3 min-h-0 flex-1 overflow-y-auto rounded-md border p-2 font-mono text-[11px] leading-5 whitespace-pre-wrap"
          >{{ logs.join('\n') || '暂无日志' }}</pre>
        </section>
      </div>

      <!-- 固定左下角：关于 -->
      <div class="border-border shrink-0 border-t px-3 py-2">
        <Button variant="ghost" size="sm" class="text-muted-foreground w-full justify-start" @click="showAbout = true">
          <InfoIcon />
          关于
        </Button>
      </div>
    </aside>

    <!-- 关于弹窗 -->
    <Teleport to="body">
      <div
        v-if="showAbout"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
        @click.self="showAbout = false"
      >
        <div class="bg-card text-card-foreground w-full max-w-sm rounded-xl border p-5 shadow-lg">
          <div class="mb-4 flex items-start justify-between gap-3">
            <div class="flex items-center gap-3">
              <img :src="logoUrl" alt="logo" class="size-10 rounded-lg object-contain invert" />
              <div>
                <h2 class="text-base font-semibold">图像超分</h2>
                <p class="text-muted-foreground text-xs">Upscaler</p>
              </div>
            </div>
            <Button variant="ghost" size="icon" class="size-8 shrink-0" @click="showAbout = false">
              <XIcon class="size-4" />
            </Button>
          </div>
          <dl class="space-y-2.5 text-sm">
            <div class="flex justify-between gap-4">
              <dt class="text-muted-foreground shrink-0">版本</dt>
              <dd>1.0.0</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-muted-foreground shrink-0">归属</dt>
              <dd class="text-right">白昼寻星工作室</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-muted-foreground shrink-0">邮箱</dt>
              <dd class="text-right break-all">xinyong.w@gmail.com</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-muted-foreground shrink-0">引擎</dt>
              <dd class="text-right">upscayl-bin</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-muted-foreground shrink-0">技术</dt>
              <dd class="text-right">Wails + Vue + shadcn-vue</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-muted-foreground shrink-0">许可</dt>
              <dd>GPL-3.0</dd>
            </div>
          </dl>
          <Button class="mt-5 w-full" @click="showAbout = false">关闭</Button>
        </div>
      </div>
    </Teleport>

    <!-- 右侧对比主区 -->
    <main class="flex min-w-0 flex-1 flex-col">
      <div class="flex h-11 shrink-0 items-center justify-between gap-3 border-b px-4">
        <div class="flex min-w-0 items-center gap-2">
          <p class="text-sm font-medium">前后对比</p>
          <Badge :variant="statusVariant">{{ statusLabel }}</Badge>
        </div>
        <NativeSelect
          v-if="comparePairs.length > 1"
          v-model="selectedPair"
          class="max-w-[220px] shrink-0"
        >
          <NativeSelectOption
            v-for="pair in comparePairs"
            :key="pair.before"
            :value="pair.before"
          >
            {{ pair.name }}
          </NativeSelectOption>
        </NativeSelect>
      </div>

      <div class="bg-muted/20 flex min-h-0 flex-1 p-4">
        <div
          v-if="compareLoading"
          class="text-muted-foreground flex size-full items-center justify-center rounded-lg border text-sm"
        >
          加载预览…
        </div>
        <ImageCompare
          v-else-if="canCompare"
          class="size-full"
          :before-src="beforeSrc"
          :after-src="afterSrc"
        />
        <div
          v-else
          class="text-muted-foreground flex size-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-8 text-center"
        >
          <ImageIcon class="size-10 opacity-40" />
          <p class="text-sm">{{ compareError || '从左侧拖入图片并开始超分' }}</p>
        </div>
      </div>
    </main>
  </div>
</template>
