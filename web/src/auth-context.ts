import { createContext, useContext } from 'react'
import type { UIIdentity } from './api/client'

export const AuthContext = createContext<UIIdentity | null>(null)
export const useIdentity = () => useContext(AuthContext)
