import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  AddStickerInput,
  addStickerToPack,
  CreatePackInput,
  createPack,
  deletePack,
  listPacks,
  removeSticker,
} from '@/features/packs/storage';

const PACKS_QUERY_KEY = ['local-packs'];

/** Local-only "query" over packs.json — no network involved, but modeled
 * as a query so screens re-read it the same way they read server data,
 * and mutations can invalidate it to refresh every screen at once. */
export function usePacks() {
  return useQuery({ queryKey: PACKS_QUERY_KEY, queryFn: () => listPacks() });
}

export function useCreatePack() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreatePackInput) => Promise.resolve(createPack(input)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: PACKS_QUERY_KEY }),
  });
}

export function useAddStickerToPack() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AddStickerInput) => addStickerToPack(input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: PACKS_QUERY_KEY }),
  });
}

export function useDeletePack() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (identifier: string) => Promise.resolve(deletePack(identifier)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: PACKS_QUERY_KEY }),
  });
}

export function useRemoveSticker() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (args: { packId: string; imageFile: string }) =>
      Promise.resolve(removeSticker(args.packId, args.imageFile)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: PACKS_QUERY_KEY }),
  });
}
