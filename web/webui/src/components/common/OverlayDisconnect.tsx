import { WifiOff } from 'lucide-react'

interface OverlayDisconnectProps {
  onReconnect: () => void
}

export default function OverlayDisconnect({ onReconnect }: OverlayDisconnectProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="mx-4 max-w-md rounded-xl bg-white p-8 shadow-xl dark:bg-slate-800">
        <div className="flex flex-col items-center gap-4 text-center">
          <WifiOff className="h-16 w-16 text-red-500" />
          <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-100">
            Connectivity to aerolab lost!
          </h2>
          <p className="text-slate-600 dark:text-slate-400">
            The connection to the AeroLab backend has been lost. Please check your
            connection and try again.
          </p>
          <button
            type="button"
            onClick={onReconnect}
            className="rounded-lg bg-blue-600 px-4 py-2 font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            Reconnect
          </button>
        </div>
      </div>
    </div>
  )
}
