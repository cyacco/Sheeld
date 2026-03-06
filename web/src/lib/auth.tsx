"use client";

import React, { createContext, useContext, useState, useEffect, useCallback } from "react";
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

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>({
    user: null,
    token: null,
    loading: true,
  });

  useEffect(() => {
    const token = localStorage.getItem("token");
    const userStr = localStorage.getItem("user");
    if (token && userStr) {
      try {
        const user = JSON.parse(userStr) as User;
        setState({ user, token, loading: false });
      } catch {
        localStorage.removeItem("token");
        localStorage.removeItem("user");
        setState({ user: null, token: null, loading: false });
      }
    } else {
      setState({ user: null, token: null, loading: false });
    }
  }, []);

  // Proactive token refresh every 30 minutes
  useEffect(() => {
    if (!state.token) return;

    const interval = setInterval(async () => {
      try {
        const result = await api.refreshToken();
        localStorage.setItem("token", result.token);
        setState((prev) => ({ ...prev, token: result.token }));
      } catch {
        // 401 will be handled by the interceptor in api.ts
      }
    }, 30 * 60 * 1000);

    return () => clearInterval(interval);
  }, [state.token]);

  const loginFn = useCallback(async (email: string, password: string) => {
    const result = await api.login(email, password);
    localStorage.setItem("token", result.token);
    localStorage.setItem("user", JSON.stringify(result.user));
    setState({ user: result.user, token: result.token, loading: false });
  }, []);

  const registerFn = useCallback(
    async (orgName: string, email: string, password: string) => {
      const result = await api.register(orgName, email, password);
      localStorage.setItem("token", result.token);
      localStorage.setItem("user", JSON.stringify(result.user));
      setState({ user: result.user, token: result.token, loading: false });
    },
    [],
  );

  const logout = useCallback(() => {
    localStorage.removeItem("token");
    localStorage.removeItem("user");
    setState({ user: null, token: null, loading: false });
  }, []);

  return (
    <AuthContext.Provider
      value={{ ...state, login: loginFn, register: registerFn, logout }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within an AuthProvider");
  return ctx;
}
