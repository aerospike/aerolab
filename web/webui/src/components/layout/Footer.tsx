import { getConfig } from '@/utils/config'

export default function Footer() {
  const config = getConfig()

  return (
    <footer className="flex h-10 items-center justify-center border-t border-slate-200 bg-white px-4 text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400">
      Powered by AeroLab · v{config.version}
    </footer>
  )
}
