import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN'
import en from './en'

export type AppLocale = 'zh-CN' | 'en'

export const localeOptions: { value: AppLocale; label: string }[] = [
  { value: 'zh-CN', label: '简体中文' },
  { value: 'en', label: 'English' },
]

function detect(): AppLocale {
  const saved = localStorage.getItem('zp-rdm-lang')
  if (saved === 'zh-CN' || saved === 'en') return saved
  return navigator.language.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: detect(),
  fallbackLocale: 'zh-CN',
  messages: { 'zh-CN': zhCN, en },
})

export function setLocale(l: AppLocale) {
  i18n.global.locale.value = l
  localStorage.setItem('zp-rdm-lang', l)
}

export function currentLocale(): AppLocale {
  return i18n.global.locale.value as AppLocale
}
