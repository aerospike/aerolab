import { useQuery } from '@tanstack/react-query'
import { fetchCommands, fetchCommandInfo } from '@/api/client'

const FIVE_MINUTES = 5 * 60 * 1000

export function useCommandTree() {
  return useQuery({
    queryKey: ['commands'],
    queryFn: fetchCommands,
    staleTime: FIVE_MINUTES,
  })
}

export function useCommandInfo(path: string | undefined) {
  return useQuery({
    queryKey: ['command', path],
    queryFn: () => fetchCommandInfo(path!),
    enabled: !!path,
  })
}
