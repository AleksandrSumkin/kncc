import React, { createContext, useContext, useEffect, useState } from 'react';
import { api } from '../api';
import { User } from '../types';

interface AuthContextType {
  user: User | null;
  loading: boolean;
  login: (token: string, user: User) => void;
  logout: () => void;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  loading: true,
  login: () => {},
  logout: () => {},
  refreshUser: async () => {},
});

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchUser = async () => {
    // 1. Check if token was supplied in URL (e.g. redirected from OIDC /?token=...)
    const urlParams = new URLSearchParams(window.location.search);
    const urlToken = urlParams.get('token');
    if (urlToken) {
      localStorage.setItem('kncc_token', urlToken);
      urlParams.delete('token');
      const cleanSearch = urlParams.toString() ? `?${urlParams.toString()}` : '';
      window.history.replaceState({}, document.title, window.location.pathname + cleanSearch + window.location.hash);
    }

    const token = localStorage.getItem('kncc_token');
    if (!token) {
      setUser(null);
      setLoading(false);
      return;
    }

    try {
      const res = await api.getMe();
      setUser(res.user);
    } catch {
      localStorage.removeItem('kncc_token');
      setUser(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchUser();

    const handleUnauthorized = () => {
      window.location.hash = '/';
      setUser(null);
    };
    window.addEventListener('kncc_unauthorized', handleUnauthorized);
    return () => window.removeEventListener('kncc_unauthorized', handleUnauthorized);
  }, []);

  const login = (token: string, u: User) => {
    localStorage.setItem('kncc_token', token);
    setUser(u);
  };

  const logout = async () => {
    try {
      await api.logout();
    } catch (e) {
      console.error(e);
    }
    localStorage.removeItem('kncc_token');
    window.location.hash = '/';
    setUser(null);
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, refreshUser: fetchUser }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => useContext(AuthContext);
