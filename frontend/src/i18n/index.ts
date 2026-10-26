import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN'
import zhTW from './zh-TW'
import en from './en'
import ja from './ja'

export type AppLocale = 'zh-CN' | 'zh-TW' | 'en' | 'ja'

export const localeOptions: { value: AppLocale; label: string }[] = [
  { value: 'zh-CN', label: '简体中文' },
  { value: 'zh-TW', label: '繁體中文' },
  { value: 'en', label: 'English' },
  { value: 'ja', label: '日本語' },
]

const LOCALES: AppLocale[] = ['zh-CN', 'zh-TW', 'en', 'ja']

function detect(): AppLocale {
  const saved = localStorage.getItem('zp-rdm-lang')
  if (LOCALES.includes(saved as AppLocale)) return saved as AppLocale
  const nav = navigator.language.toLowerCase()
  if (nav.startsWith('zh')) {
    // zh-TW / zh-HK / zh-Hant → 繁体；其余中文 → 简体
    return /tw|hk|mo|hant/.test(nav) ? 'zh-TW' : 'zh-CN'
  }
  if (nav.startsWith('ja')) return 'ja'
  return 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: detect(),
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, 'zh-TW': zhTW, en, ja },
})

export function setLocale(l: AppLocale) {
  i18n.global.locale.value = l
  localStorage.setItem('zp-rdm-lang', l)
}

export function currentLocale(): AppLocale {
  return i18n.global.locale.value as AppLocale
}
