import { Navigate, Route, Routes } from 'react-router'
import AuthGuard from './components/AuthGuard'
import Login from './pages/Login'
import Settings from './pages/Settings'

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/settings"
        element={
          <AuthGuard>
            <Settings />
          </AuthGuard>
        }
      />
      <Route path="*" element={<Navigate to="/settings" replace />} />
    </Routes>
  )
}
