import { useEffect, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { catalogSchema, customerPageSchema } from "./domain";

export function useDebounced<T>(value: T, delay = 250) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

// nextStepLabel shows the step agreed with the customer for an opportunity without a date.
export function useNextStepLabel() {
  const catalog = useCatalog();
  return (code: string) =>
    catalog.data?.nextActions.find((a) => a.code === code)?.label ?? code;
}

// Visit reasons, next actions and product categories are configured on the server.
export function useCatalog() {
  return useQuery({
    queryKey: ["catalog"],
    queryFn: async () => catalogSchema.parse(await api("catalog")),
    staleTime: 60 * 60 * 1000,
  });
}

export type CustomerQuery = {
  q?: string;
  owner?: string;
  ownership?: string;
  sort?: string;
  limit?: number;
};

export function useCustomers(query: CustomerQuery, enabled = true) {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query))
    if (v !== undefined && v !== "") params.set(k, String(v));
  return useQuery({
    queryKey: ["customers", params.toString()],
    queryFn: async () =>
      customerPageSchema.parse(await api("customers?" + params.toString())),
    enabled,
    placeholderData: (previous) => previous,
  });
}

// useCustomerPages loads the directory page by page for "show more" lists.
export function useCustomerPages(query: Omit<CustomerQuery, "limit">) {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query))
    if (v !== undefined && v !== "") params.set(k, String(v));
  return useInfiniteQuery({
    queryKey: ["customers", "pages", params.toString()],
    initialPageParam: 0,
    queryFn: async ({ pageParam }) =>
      customerPageSchema.parse(
        await api(
          `customers?${params.toString()}&limit=24&offset=${pageParam}`,
        ),
      ),
    getNextPageParam: (last) =>
      last.offset + last.items.length < last.total
        ? last.offset + last.items.length
        : undefined,
    placeholderData: (previous) => previous,
  });
}
