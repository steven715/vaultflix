import { useState, useEffect } from 'react'
import { listUsers, createUser, deleteUser, enableUser, resetUserPassword } from '../../api/admin'
import type { User } from '../../types'
import Header from '../../components/Header'

export default function UserManagePage() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)

  // Create user form
  const [showCreate, setShowCreate] = useState(false)
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newRole, setNewRole] = useState('viewer')
  const [creating, setCreating] = useState(false)

  // Reset password modal
  const [resetTarget, setResetTarget] = useState<User | null>(null)
  const [resetPass, setResetPass] = useState('')
  const [resetting, setResetting] = useState(false)

  // Delete confirm
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

  const handleCreate = async () => {
    if (!newUsername || !newPassword) return
    setCreating(true)
    try {
      await createUser(newUsername, newPassword, newRole)
      const data = await listUsers()
      setUsers(data)
      setShowCreate(false)
      setNewUsername('')
      setNewPassword('')
      setNewRole('viewer')
    } catch (err) {
      alert(err instanceof Error ? err.message : 'failed to create user')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteUser(id)
      setUsers(users.map(u => u.id === id ? { ...u, disabled_at: new Date().toISOString() } : u))
      setDeleteTarget(null)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'failed to disable user')
    }
  }

  const handleEnable = async (id: string) => {
    try {
      await enableUser(id)
      setUsers(users.map(u => u.id === id ? { ...u, disabled_at: null } : u))
    } catch (err) {
      alert(err instanceof Error ? err.message : 'failed to enable user')
    }
  }

  const handleResetPassword = async () => {
    if (!resetTarget || !resetPass) return
    setResetting(true)
    try {
      await resetUserPassword(resetTarget.id, resetPass)
      setResetTarget(null)
      setResetPass('')
      alert('password updated')
    } catch (err) {
      alert(err instanceof Error ? err.message : 'failed to reset password')
    } finally {
      setResetting(false)
    }
  }

  return (
    <div style={{ minHeight: '100vh', backgroundColor: '#0f0f0f', color: '#fff' }}>
      <Header searchQuery="" onSearch={() => {}} />
      <div style={{ maxWidth: 900, margin: '0 auto', padding: '2rem 1rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <h1 style={{ fontSize: '1.5rem', margin: 0 }}>User Management</h1>
          <button
            onClick={() => setShowCreate(true)}
            style={{ padding: '0.5rem 1rem', backgroundColor: '#e50914', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
          >
            Create User
          </button>
        </div>

        {loading ? (
          <p>Loading...</p>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #333', textAlign: 'left' }}>
                <th style={{ padding: '0.75rem' }}>Username</th>
                <th style={{ padding: '0.75rem' }}>Role</th>
                <th style={{ padding: '0.75rem' }}>Status</th>
                <th style={{ padding: '0.75rem' }}>Created</th>
                <th style={{ padding: '0.75rem' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map(user => (
                <tr key={user.id} style={{ borderBottom: '1px solid #222' }}>
                  <td style={{ padding: '0.75rem' }}>{user.username}</td>
                  <td style={{ padding: '0.75rem' }}>
                    <span style={{
                      padding: '0.2rem 0.5rem',
                      borderRadius: 4,
                      fontSize: '0.85rem',
                      backgroundColor: user.role === 'admin' ? '#b45309' : '#1e40af',
                    }}>
                      {user.role}
                    </span>
                  </td>
                  <td style={{ padding: '0.75rem' }}>
                    {user.disabled_at ? (
                      <span style={{ color: '#ef4444' }}>Disabled</span>
                    ) : (
                      <span style={{ color: '#22c55e' }}>Active</span>
                    )}
                  </td>
                  <td style={{ padding: '0.75rem' }}>{new Date(user.created_at).toLocaleDateString()}</td>
                  <td style={{ padding: '0.75rem' }}>
                    <button
                      onClick={() => setResetTarget(user)}
                      style={{ marginRight: '0.5rem', padding: '0.3rem 0.6rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                    >
                      Reset Password
                    </button>
                    {user.role !== 'admin' && !user.disabled_at && (
                      <button
                        onClick={() => setDeleteTarget(user)}
                        style={{ padding: '0.3rem 0.6rem', backgroundColor: '#7f1d1d', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                      >
                        Disable
                      </button>
                    )}
                    {user.role !== 'admin' && user.disabled_at && (
                      <button
                        onClick={() => handleEnable(user.id)}
                        style={{ padding: '0.3rem 0.6rem', backgroundColor: '#15803d', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                      >
                        Enable
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {/* Create User Modal */}
        {showCreate && (
          <div style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }}>
            <div style={{ backgroundColor: '#1a1a1a', padding: '2rem', borderRadius: 8, width: 400 }}>
              <h2 style={{ marginTop: 0 }}>Create User</h2>
              <div style={{ marginBottom: '1rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>Username</label>
                <input
                  value={newUsername}
                  onChange={e => setNewUsername(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4, boxSizing: 'border-box' }}
                />
              </div>
              <div style={{ marginBottom: '1rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>Password</label>
                <input
                  type="password"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4, boxSizing: 'border-box' }}
                />
              </div>
              <div style={{ marginBottom: '1.5rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>Role</label>
                <select
                  value={newRole}
                  onChange={e => setNewRole(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4 }}
                >
                  <option value="viewer">Viewer</option>
                  <option value="admin">Admin</option>
                </select>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
                <button
                  onClick={() => setShowCreate(false)}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleCreate}
                  disabled={creating || !newUsername || !newPassword}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#e50914', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', opacity: creating ? 0.5 : 1 }}
                >
                  {creating ? 'Creating...' : 'Create'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Reset Password Modal */}
        {resetTarget && (
          <div style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }}>
            <div style={{ backgroundColor: '#1a1a1a', padding: '2rem', borderRadius: 8, width: 400 }}>
              <h2 style={{ marginTop: 0 }}>Reset Password</h2>
              <p style={{ color: '#aaa' }}>User: {resetTarget.username}</p>
              <div style={{ marginBottom: '1.5rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>New Password</label>
                <input
                  type="password"
                  value={resetPass}
                  onChange={e => setResetPass(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4, boxSizing: 'border-box' }}
                />
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
                <button
                  onClick={() => { setResetTarget(null); setResetPass('') }}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleResetPassword}
                  disabled={resetting || !resetPass}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#e50914', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', opacity: resetting ? 0.5 : 1 }}
                >
                  {resetting ? 'Updating...' : 'Update Password'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Delete Confirm Modal */}
        {deleteTarget && (
          <div style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }}>
            <div style={{ backgroundColor: '#1a1a1a', padding: '2rem', borderRadius: 8, width: 400 }}>
              <h2 style={{ marginTop: 0 }}>Confirm Disable</h2>
              <p>Are you sure you want to disable <strong>{deleteTarget.username}</strong>?</p>
              <p style={{ color: '#aaa', fontSize: '0.9rem' }}>The user will not be able to log in. Their data (favorites, watch history) will be preserved.</p>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
                <button
                  onClick={() => setDeleteTarget(null)}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Cancel
                </button>
                <button
                  onClick={() => handleDelete(deleteTarget.id)}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#dc2626', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Disable User
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
