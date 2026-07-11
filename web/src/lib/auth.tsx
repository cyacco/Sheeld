"use client";

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
} from "react";
import type { User } from "./types";
import * as api from "./api";

interface AuthState {
  user: User | null;
  token: string | null;
  loading: boolean;
}

interface AuthContextValue extends AuthState {
  login: (email: string, password: string) => Promise<void>;
  register: (orgName: string, email: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

// Auth is persisted in localStorage and read via useSyncExternalStore so the
// initial value comes from the store during render (with an SSR-safe "loading"
// server snapshot) instead of a setState-in-effect on mount. getSnapshot must
// return a stable reference between changes, so the snapshot is cached and only
// recomputed when the stored credentials actually change.
const LOADING: AuthState = { user: null, token: null, loading: true };
const LOGGED_OUT: AuthState = { user: null, token: null, loading: false };

let snapshot: AuthState = LOADING;
let hydrated = false;
const listeners = new Set<() => void>();

function readPersistedAuth(): AuthState {
  const token = localStorage.getItem("token");
  const userStr = localStorage.getItem("user");
  if (token && userStr) {
    try {
      return { user: JSON.parse(userStr) as User, token, loading: false };
    } catch {
      localStorage.removeItem("token");
      localStorage.removeItem("user");
    }
  }
  return LOGGED_OUT;
}

function emit() {
  listeners.forEach((l) => l());
}

// setPersistedAuth writes the credentials to localStorage and the in-memory
// snapshot, then notifies subscribers. Passing null clears them (logout).
function setPersistedAuth(next: { user: User; token: string } | null) {
  if (next) {
    localStorage.setItem("token", next.token);
    localStorage.setItem("user", JSON.stringify(next.user));
    snapshot = { user: next.user, token: next.token, loading: false };
  } else {
    localStorage.removeItem("token");
    localStorage.removeItem("user");
    snapshot = LOGGED_OUT;
  }
  emit();
}

// setToken refreshes just the token (proactive refresh keeps the user/session).
function setToken(token: string) {
  localStorage.setItem("token", token);
  snapshot = { ...snapshot, token };
  emit();
}

function subscribe(cb: () => void): () => void {
  if (!hydrated) {
    hydrated = true;
    snapshot = readPersistedAuth();
  }
  listeners.add(cb);
  const onStorage = (e: StorageEvent) => {
    if (e.key === "token" || e.key === "user") {
      snapshot = readPersistedAuth();
      emit();
    }
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(cb);
    window.removeEventListener("storage", onStorage);
  };
}

function getSnapshot(): AuthState {
  return snapshot;
}

function getServerSnapshot(): AuthState {
  return LOADING;
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const state = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  // Proactive token refresh every 30 minutes
  useEffect(() => {
    if (!state.token) return;

    const interval = setInterval(async () => {
      try {
        const result = await api.refreshToken();
        setToken(result.token);
      } catch {
        // 401 will be handled by the interceptor in api.ts
      }
    }, 30 * 60 * 1000);

    return () => clearInterval(interval);
  }, [state.token]);

  const loginFn = useCallback(async (email: string, password: string) => {
    const result = await api.login(email, password);
    setPersistedAuth({ user: result.user, token: result.token });
  }, []);

  const registerFn = useCallback(
    async (orgName: string, email: string, password: string) => {
      const result = await api.register(orgName, email, password);
      setPersistedAuth({ user: result.user, token: result.token });
    },
    [],
  );

  const logout = useCallback(() => {
    setPersistedAuth(null);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({ ...state, login: loginFn, register: registerFn, logout }),
    [state, loginFn, registerFn, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within an AuthProvider");
  return ctx;
}
