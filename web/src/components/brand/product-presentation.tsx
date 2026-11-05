import { createContext, useContext } from "react";
import { useAppearanceStore } from "@/stores/use-appearance-store";

export const clientName = () => useAppearanceStore.getState().appearance.brandName;
export const useClientName = () => useAppearanceStore((state) => state.appearance.brandName);
export function ClientName() { return <>{useClientName()}</>; }

// Optional distribution presentation; the local product retains its defaults.
export const ProductPresentationContext = createContext<{
    contacts?: { website: string; qrImage: string; openWebsite: () => void };
    staticVersion?: boolean;
    hideBuiltinChannel?: boolean;
    hideCliInstallInstructions?: boolean;
}>({});
export const useProductPresentation = () => useContext(ProductPresentationContext);
