import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchInventory,
  fetchInventorySchema,
  inventoryAction,
  type InventoryActionRequest,
  type InventorySchema,
} from '@/api/client'

export function useInventory(type: string) {
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['inventory', type],
    queryFn: () => fetchInventory(type),
    enabled: !!type,
    staleTime: 5000,
    // Poll all tabs every 30s to pick up external changes
    // (backend refreshes from cloud APIs on its own interval)
    refetchInterval: 30000,
  })

  return {
    data: (data ?? []) as Record<string, unknown>[],
    isLoading,
    error,
    refetch,
  }
}

export function useInventorySchema() {
  const { data, isLoading } = useQuery({
    queryKey: ['inventory', 'schema'],
    queryFn: () => fetchInventorySchema(),
    staleTime: 60000,
  })

  return {
    schema: data as InventorySchema | undefined,
    backend: (data as InventorySchema | undefined)?.backend ?? 'docker',
    isLoading,
  }
}

export function useInventoryAction() {
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: (body: InventoryActionRequest) => inventoryAction(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['inventory'] })
    },
  })

  const mutateAsync = async (body: InventoryActionRequest) => {
    const result = await mutation.mutateAsync(body)
    return { jobId: result.jobId, jobIds: result.jobIds }
  }

  return {
    mutateAsync,
    isPending: mutation.isPending,
    error: mutation.error,
  }
}
