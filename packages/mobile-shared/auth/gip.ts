import auth, { type FirebaseAuthTypes } from "@react-native-firebase/auth";

export interface GIPAuthConfig {
  tenantId: string;
}

export function createGIPAuth(config: GIPAuthConfig) {
  const firebaseAuth = auth();
  firebaseAuth.tenantId = config.tenantId;

  return {
    signIn: (email: string, password: string) =>
      firebaseAuth.signInWithEmailAndPassword(email, password),
    signOut: () => firebaseAuth.signOut(),
    getIdToken: async (): Promise<string | null> => {
      const user = firebaseAuth.currentUser;
      if (!user) return null;
      return user.getIdToken(false);
    },
    getIdTokenForced: async (): Promise<string | null> => {
      const user = firebaseAuth.currentUser;
      if (!user) return null;
      return user.getIdToken(true);
    },
    getCurrentUser: (): FirebaseAuthTypes.User | null => firebaseAuth.currentUser,
    onAuthStateChanged: (callback: (user: FirebaseAuthTypes.User | null) => void) =>
      firebaseAuth.onAuthStateChanged(callback),
    sendPasswordResetEmail: (email: string) =>
      firebaseAuth.sendPasswordResetEmail(email),
  };
}

export type GIPAuth = ReturnType<typeof createGIPAuth>;
