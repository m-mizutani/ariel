import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { fetchMe, type MeResult } from '../api'

export type AuthState = { kind: 'loading' } | MeResult

type AuthContextValue = {
  state: AuthState
  reload: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ kind: 'loading' })

  const reload = useCallback(async () => {
    setState({ kind: 'loading' })
    setState(await fetchMe())
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  return <AuthContext.Provider value={{ state, reload }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
