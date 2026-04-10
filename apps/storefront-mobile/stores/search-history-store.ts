import { create } from "zustand";

const MAX_SEARCHES = 10;

interface SearchHistoryState {
  searches: string[];
  addSearch: (term: string) => void;
  removeSearch: (term: string) => void;
  clear: () => void;
}

export const useSearchHistoryStore = create<SearchHistoryState>()((set) => ({
  searches: [],

  addSearch: (term) =>
    set((state) => {
      const trimmed = term.trim();
      if (!trimmed) return state;
      const filtered = state.searches.filter((s) => s !== trimmed);
      return { searches: [trimmed, ...filtered].slice(0, MAX_SEARCHES) };
    }),

  removeSearch: (term) =>
    set((state) => ({
      searches: state.searches.filter((s) => s !== term),
    })),

  clear: () => set({ searches: [] }),
}));
