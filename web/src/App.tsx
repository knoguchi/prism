import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom'
import { Radio, FileCode, Settings, CheckCircle } from 'lucide-react'
import TrafficPage from './pages/TrafficPage'
import SchemasPage from './pages/SchemasPage'
import ValidationPage from './pages/ValidationPage'
import SettingsPage from './pages/SettingsPage'

function App() {
  return (
    <BrowserRouter>
      <div className="h-screen flex flex-col bg-proxy-bg text-proxy-text">
        {/* Global Navigation */}
        <nav className="h-12 flex items-center px-4 border-b border-proxy-border bg-proxy-sidebar">
          {/* Logo */}
          <div className="flex items-center gap-2 font-semibold mr-8">
            <span className="text-proxy-accent">Prism</span>
          </div>

          {/* Tabs */}
          <div className="flex items-center gap-1">
            <NavLink
              to="/"
              className={({ isActive }) =>
                `flex items-center gap-2 px-4 py-2 rounded text-sm ${
                  isActive
                    ? 'bg-proxy-accent text-white'
                    : 'hover:bg-proxy-border'
                }`
              }
            >
              <Radio size={16} />
              Traffic
            </NavLink>
            <NavLink
              to="/schemas"
              className={({ isActive }) =>
                `flex items-center gap-2 px-4 py-2 rounded text-sm ${
                  isActive
                    ? 'bg-proxy-accent text-white'
                    : 'hover:bg-proxy-border'
                }`
              }
            >
              <FileCode size={16} />
              Schemas
            </NavLink>
            <NavLink
              to="/validation"
              className={({ isActive }) =>
                `flex items-center gap-2 px-4 py-2 rounded text-sm ${
                  isActive
                    ? 'bg-proxy-accent text-white'
                    : 'hover:bg-proxy-border'
                }`
              }
            >
              <CheckCircle size={16} />
              Validation
            </NavLink>
            <NavLink
              to="/settings"
              className={({ isActive }) =>
                `flex items-center gap-2 px-4 py-2 rounded text-sm ${
                  isActive
                    ? 'bg-proxy-accent text-white'
                    : 'hover:bg-proxy-border'
                }`
              }
            >
              <Settings size={16} />
              Settings
            </NavLink>
          </div>

          <div className="flex-1" />

          {/* Status indicator */}
          <div className="flex items-center gap-2 text-sm text-proxy-text-dim">
            <span className="w-2 h-2 rounded-full bg-emerald-500" />
            Proxy running
          </div>
        </nav>

        {/* Page content */}
        <div className="flex-1 flex overflow-hidden">
          <Routes>
            <Route path="/" element={<TrafficPage />} />
            <Route path="/schemas" element={<SchemasPage />} />
            <Route path="/validation" element={<ValidationPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Routes>
        </div>
      </div>
    </BrowserRouter>
  )
}

export default App
