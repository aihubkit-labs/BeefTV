import { createContext, useContext } from "react";
import { useClientName } from "@/lib/product-name";

export { clientName, useClientName } from "@/lib/product-name";
export function ClientName() { return <>{useClientName()}</>; }

// Optional distribution presentation; the local product retains its defaults.
export const ProductPresentationContext = createContext<{
    contacts?: { website: string; qrImage: string; openWebsite: () => void };
    staticVersion?: boolean;
    hideBuiltinChannel?: boolean;
    hideCliInstallInstructions?: boolean;
    hideDefaultUserAgent?: boolean;
}>({});
export const useProductPresentation = () => useContext(ProductPresentationContext);
