import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import zhCN from './locales/zh-CN.json'

export const appLanguage = 'zh-CN' as const

export const resources = {
  [appLanguage]: { translation: zhCN },
} as const

void i18n.use(initReactI18next).init({
  lng: appLanguage,
  resources,
  fallbackLng: appLanguage,
  supportedLngs: [appLanguage],
  load: 'currentOnly',
  keySeparator: false,
  nsSeparator: false,
  returnEmptyString: false,
  returnNull: false,
  interpolation: {
    escapeValue: false,
  },
})

if (typeof document !== 'undefined') document.documentElement.lang = appLanguage

export default i18n
