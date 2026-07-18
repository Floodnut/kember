import { createContext, type ReactNode, useContext } from "react";
import { createApiClient, type ApiClient } from "./client";

const ApiContext = createContext<ApiClient>(createApiClient());

export function ApiProvider({ client, children }: { client: ApiClient; children: ReactNode }) {
  return <ApiContext.Provider value={client}>{children}</ApiContext.Provider>;
}

export function useApi(): ApiClient {
  return useContext(ApiContext);
}
