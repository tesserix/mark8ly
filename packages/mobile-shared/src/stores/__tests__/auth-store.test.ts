import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "../auth-store";

describe("useAuthStore", () => {
  beforeEach(() => {
    useAuthStore.getState().reset();
  });

  it("has correct initial state", () => {
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.isLoading).toBe(false);
  });

  it("setUser sets user and marks authenticated", () => {
    const user = {
      uid: "user-123",
      email: "alice@example.com",
      displayName: "Alice",
    };
    useAuthStore.getState().setUser(user);

    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(true);
    expect(state.user).toEqual(user);
  });

  it("setUser with null clears user and marks unauthenticated", () => {
    useAuthStore.getState().setUser({
      uid: "user-123",
      email: "alice@example.com",
      displayName: "Alice",
    });
    useAuthStore.getState().setUser(null);

    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
  });

  it("setLoading updates isLoading", () => {
    useAuthStore.getState().setLoading(true);
    expect(useAuthStore.getState().isLoading).toBe(true);

    useAuthStore.getState().setLoading(false);
    expect(useAuthStore.getState().isLoading).toBe(false);
  });

  it("reset restores initial state", () => {
    useAuthStore.getState().setUser({
      uid: "user-123",
      email: "alice@example.com",
      displayName: "Alice",
    });
    useAuthStore.getState().setLoading(true);

    useAuthStore.getState().reset();

    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.isLoading).toBe(false);
  });

  it("user can have null displayName", () => {
    useAuthStore.getState().setUser({
      uid: "user-456",
      email: "bob@example.com",
      displayName: null,
    });
    expect(useAuthStore.getState().user?.displayName).toBeNull();
  });
});
