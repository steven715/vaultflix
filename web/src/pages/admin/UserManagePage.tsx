import { useState, useEffect } from 'react'
import { listUsers, createUser, deleteUser, enableUser, resetUserPassword } from '../../api/admin'
import { useToast } from '../../contexts/ToastContext'
import type { User } from '../../types'

export default function UserManagePage() {
  const toast = useToast()
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)

  const [showCreate, setShowCreate] = useState(false)
  const [resetTarget, setResetTarget] = useState<User | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null)

  useEffect(() => {
    let cancelled = false
    const fetchUsers = async () => {
      try {
        const data = await listUsers()
        if (!cancelled) setUsers(data)
      } catch (err) {
        console.error('failed to list users', err)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    fetchUsers()
    return () => { cancelled = true }
  }, [])

  const handleCreate = async (username: string, password: string, role: string) => {
    await createUser(username, password, role)
    const data = await listUsers()
    setUsers(data)
    setShowCreate(false)
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteUser(id)
      setUsers(users.map(u => u.id === id ? { ...u, disabled_at: new Date().toISOString() } : u))
      setDeleteTarget(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'failed to disable user')
    }
  }

  const handleEnable = async (id: string) => {
    try {
      await enableUser(id)
      setUsers(users.map(u => u.id === id ? { ...u, disabled_at: null } : u))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'failed to enable user')
    }
  }

  const handleResetPassword = async (userId: string, newPass: string) => {
    await resetUserPassword(userId, newPass)
  }

  return (
    <div className="p-7 max-w-[980px] mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="font-display font-bold text-xl tracking-tight text-cream">使用者與權限</h1>
          <p className="text-sm text-muted mt-0.5">
            <span className="font-mono">{users.length}</span> 位 · Casbin RBAC
          </p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="bg-accent text-accent-ink text-sm font-medium px-4 py-2 rounded-btn hover:brightness-110"
        >
          建立使用者
        </button>
      </div>

      {loading ? (
        <div className="text-faint text-center py-20">載入中...</div>
      ) : (
        <table className="w-full text-sm text-left">
          <thead>
            <tr className="bg-surface-2 text-muted">
              <th className="px-4 py-3 font-medium">使用者</th>
              <th className="px-4 py-3 font-medium">角色</th>
              <th className="px-4 py-3 font-medium">狀態</th>
              <th className="px-4 py-3 font-medium">建立</th>
              <th className="px-4 py-3 font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {users.map(user => (
              <tr key={user.id} className="border-b border-border hover:bg-surface-2">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2.5">
                    <div
                      className="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold text-accent-ink shrink-0"
                      style={{ background: 'linear-gradient(150deg, #FFB23F, #FF8A3D)' }}
                    >
                      {user.username[0].toUpperCase()}
                    </div>
                    <span className="text-cream">{user.username}</span>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <span className={`rounded-pill px-2.5 py-1 text-xs ${user.role === 'admin' ? 'bg-accent/15 text-accent' : 'bg-surface-2 text-muted'}`}>
                    {user.role}
                  </span>
                </td>
                <td className="px-4 py-3">
                  {user.disabled_at
                    ? <span className="text-faint">停用</span>
                    : <span className="text-live flex items-center gap-1.5"><span className="w-1.5 h-1.5 rounded-full bg-live inline-block" />啟用</span>
                  }
                </td>
                <td className="px-4 py-3 font-mono text-muted">{new Date(user.created_at).toLocaleDateString()}</td>
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <button onClick={() => setResetTarget(user)} className="text-xs text-muted hover:text-cream px-2 py-1 rounded-btn">重設密碼</button>
                    {user.role !== 'admin' && !user.disabled_at && (
                      <button onClick={() => setDeleteTarget(user)} className="text-xs text-fav hover:brightness-110 px-2 py-1 rounded-btn">停用</button>
                    )}
                    {user.role !== 'admin' && user.disabled_at && (
                      <button onClick={() => handleEnable(user.id)} className="text-xs text-live hover:brightness-110 px-2 py-1 rounded-btn">啟用</button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {showCreate && (
        <CreateUserModal
          onClose={() => setShowCreate(false)}
          onCreate={handleCreate}
          onError={(msg) => toast.error(msg)}
        />
      )}
      {resetTarget && (
        <ResetPasswordModal
          user={resetTarget}
          onClose={() => setResetTarget(null)}
          onReset={handleResetPassword}
          onSuccess={() => toast.success('已更新密碼')}
          onError={(msg) => toast.error(msg)}
        />
      )}
      {deleteTarget && (
        <DisableUserModal
          user={deleteTarget}
          onClose={() => setDeleteTarget(null)}
          onConfirm={() => handleDelete(deleteTarget.id)}
        />
      )}
    </div>
  )
}

/* ---------- Modals ---------- */

const OVERLAY = 'fixed inset-0 flex items-center justify-center z-50 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px]'
const CARD = 'bg-surface border border-border rounded-lg p-6 w-full max-w-md'
const INPUT = 'w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-3'
const BTN_CANCEL = 'text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn'
const BTN_PRIMARY = 'bg-accent text-accent-ink text-sm px-4 py-1.5 rounded-btn disabled:opacity-50 hover:brightness-110'
const BTN_DANGER = 'bg-fav text-cream text-sm px-4 py-1.5 rounded-btn hover:brightness-110'

function CreateUserModal({
  onClose, onCreate, onError,
}: { onClose: () => void; onCreate: (u: string, p: string, r: string) => Promise<void>; onError: (msg: string) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState('viewer')
  const [busy, setBusy] = useState(false)

  const handleSubmit = async () => {
    if (!username || !password) return
    setBusy(true)
    try { await onCreate(username, password, role) }
    catch (err) { onError(err instanceof Error ? err.message : 'failed to create user') }
    finally { setBusy(false) }
  }

  return (
    <div className={OVERLAY} onClick={onClose}>
      <div className={CARD} onClick={e => e.stopPropagation()}>
        <h2 className="font-display font-semibold text-lg text-cream mb-4">建立使用者</h2>
        <label className="block text-sm text-muted mb-1">使用者名稱</label>
        <input value={username} onChange={e => setUsername(e.target.value)} className={INPUT} />
        <label className="block text-sm text-muted mb-1">密碼</label>
        <input type="password" value={password} onChange={e => setPassword(e.target.value)} className={INPUT} />
        <label className="block text-sm text-muted mb-1">角色</label>
        <select value={role} onChange={e => setRole(e.target.value)} className={`${INPUT} mb-5`}>
          <option value="viewer">Viewer</option>
          <option value="admin">Admin</option>
        </select>
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className={BTN_CANCEL}>取消</button>
          <button onClick={handleSubmit} disabled={busy || !username || !password} className={BTN_PRIMARY}>
            {busy ? '建立中...' : '建立'}
          </button>
        </div>
      </div>
    </div>
  )
}

function ResetPasswordModal({
  user, onClose, onReset, onSuccess, onError,
}: { user: User; onClose: () => void; onReset: (id: string, p: string) => Promise<void>; onSuccess: () => void; onError: (msg: string) => void }) {
  const [pass, setPass] = useState('')
  const [busy, setBusy] = useState(false)

  const handleSubmit = async () => {
    if (!pass) return
    setBusy(true)
    try { await onReset(user.id, pass); onClose(); onSuccess() }
    catch (err) { onError(err instanceof Error ? err.message : 'failed to reset password') }
    finally { setBusy(false) }
  }

  return (
    <div className={OVERLAY} onClick={onClose}>
      <div className={CARD} onClick={e => e.stopPropagation()}>
        <h2 className="font-display font-semibold text-lg text-cream mb-1">重設密碼</h2>
        <p className="text-sm text-muted mb-4">使用者：{user.username}</p>
        <label className="block text-sm text-muted mb-1">新密碼</label>
        <input type="password" value={pass} onChange={e => setPass(e.target.value)} className={`${INPUT} mb-5`} />
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className={BTN_CANCEL}>取消</button>
          <button onClick={handleSubmit} disabled={busy || !pass} className={BTN_PRIMARY}>
            {busy ? '更新中...' : '更新密碼'}
          </button>
        </div>
      </div>
    </div>
  )
}

function DisableUserModal({ user, onClose, onConfirm }: { user: User; onClose: () => void; onConfirm: () => void }) {
  return (
    <div className={OVERLAY} onClick={onClose}>
      <div className="bg-surface border border-border rounded-lg p-6 w-full max-w-sm" onClick={e => e.stopPropagation()}>
        <h2 className="font-display font-semibold text-lg text-cream mb-2">確認停用</h2>
        <p className="text-sm text-muted mb-1">確定要停用 <strong className="text-cream">{user.username}</strong>？</p>
        <p className="text-xs text-faint mb-5">停用後使用者無法登入，收藏與觀看紀錄將保留。</p>
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className={BTN_CANCEL}>取消</button>
          <button onClick={onConfirm} className={BTN_DANGER}>確認停用</button>
        </div>
      </div>
    </div>
  )
}
