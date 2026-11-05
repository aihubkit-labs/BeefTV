import { useAppearanceStore } from "@/stores/use-appearance-store";

// Read at presentation time so a refreshed distribution name is not cached.
export const clientName = () => useAppearanceStore.getState().appearance.brandName;
export const useClientName = () => useAppearanceStore((state) => state.appearance.brandName);
