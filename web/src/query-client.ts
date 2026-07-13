import { QueryClient } from "@tanstack/react-query"

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 15_000, retry: (count, error) => !(error instanceof Error && "status" in error && error.status === 401) && count < 2 },
    mutations: { retry: false },
  },
})
