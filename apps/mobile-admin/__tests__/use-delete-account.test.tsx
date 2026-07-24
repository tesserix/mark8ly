import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useDeleteAccount } from "@/lib/admin-api/account-actions";

jest.mock("@repo/mobile-shared/api/account", () => {
  const mockDeleteAccount = jest.fn();
  return {
    createAccountApi: () => ({
      deleteAccount: mockDeleteAccount,
    }),
    __mockDeleteAccount: mockDeleteAccount,
  };
});

jest.mock("@repo/mobile-shared/auth/provider", () => {
  const mockSignOut = jest.fn();
  const mockRefreshToken = jest.fn();
  return {
    useAuth: () => ({
      signOut: mockSignOut,
      refreshToken: mockRefreshToken,
    }),
    __mockSignOut: mockSignOut,
    __mockRefreshToken: mockRefreshToken,
  };
});

jest.mock("@/lib/api-client", () => {
  return {
    useApiClient: () => ({}),
  };
});

// eslint-disable-next-line @typescript-eslint/no-var-requires
const { __mockDeleteAccount } = require("@repo/mobile-shared/api/account") as {
  __mockDeleteAccount: jest.Mock;
};
// eslint-disable-next-line @typescript-eslint/no-var-requires
const { __mockSignOut, __mockRefreshToken } = require("@repo/mobile-shared/auth/provider") as {
  __mockSignOut: jest.Mock;
  __mockRefreshToken: jest.Mock;
};

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("useDeleteAccount", () => {
  beforeEach(() => {
    __mockDeleteAccount.mockReset();
    __mockSignOut.mockReset();
    __mockRefreshToken.mockReset();
  });

  it("refreshes the token, deletes the account, then signs out — in that order", async () => {
    const callOrder: string[] = [];
    __mockRefreshToken.mockImplementation(async () => {
      callOrder.push("refreshToken");
      return "fresh-token";
    });
    __mockDeleteAccount.mockImplementation(async () => {
      callOrder.push("deleteAccount");
    });
    __mockSignOut.mockImplementation(async () => {
      callOrder.push("signOut");
    });

    const { result } = renderHook(() => useDeleteAccount(), { wrapper });
    result.current.mutate();

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(callOrder).toEqual(["refreshToken", "deleteAccount", "signOut"]);
  });

  it("does not sign out and surfaces the error when deleteAccount rejects", async () => {
    __mockRefreshToken.mockResolvedValue("fresh-token");
    const deleteError = new Error("delete failed");
    __mockDeleteAccount.mockRejectedValue(deleteError);

    const { result } = renderHook(() => useDeleteAccount(), { wrapper });
    result.current.mutate();

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBe(deleteError);
    expect(__mockSignOut).not.toHaveBeenCalled();
  });
});
