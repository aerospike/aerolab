import { useQuery } from '@tanstack/react-query'
import { fetchHealth } from '@/api/client'
import { useMemo } from 'react'

const POLL_INTERVAL = 15 * 1000

export function useHealth() {
  const { data, refetch, failureCount, isSuccess } = useQuery({
    queryKey: ['health'],
    queryFn: fetchHealth,
    refetchInterval: POLL_INTERVAL,
    retry: false,
    staleTime: 0,
  })

  const isHealthy = useMemo(() => {
    if (failureCount >= 3) return false
    return isSuccess
  }, [failureCount, isSuccess])

  return {
    isHealthy,
    failureCount,
    version: data?.version ?? '',
    allowServerBrowse: data?.allowServerBrowse,
    refetch,
  }
}
