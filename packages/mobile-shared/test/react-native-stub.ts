// Minimal react-native stub for vitest (mark8ly#1000).
//
// useSupportChat imports AppState to pause/resume the socket with the app
// lifecycle. Vitest cannot transform react-native's Flow-typed source, so
// tests alias the module here. Only what the code under test touches is
// implemented — add to it rather than reaching for a heavier RN preset.

type Handler = (state: string) => void;

export type AppStateStatus = "active" | "background" | "inactive";

export const AppState = {
  currentState: "active" as AppStateStatus,
  addEventListener(_event: string, _handler: Handler) {
    return { remove() {} };
  },
};
