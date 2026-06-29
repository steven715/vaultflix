import { Outlet } from 'react-router-dom'
import AdminSidebar from './AdminSidebar'
import AdminTopbar from './AdminTopbar'

export default function AdminLayout() {
  return (
    <div className="min-h-screen bg-bg text-cream">
      <AdminSidebar />
      <div className="pl-[78px] flex flex-col min-h-screen">
        <AdminTopbar />
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
