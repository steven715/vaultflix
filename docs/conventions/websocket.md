# WebSocket 慣例

> 動 `internal/websocket/` 或 `web/src/hooks/useWebSocket.ts` 前先讀。

## Hub Pattern

- WebSocket 連線管理集中在 `internal/websocket/` package
- `Hub` struct 透過 channel 序列化所有 client 註冊/移除/訊息推送，避免 map 並發存取
- 支援 per-user targeted message（`SendToUser`）和全域 broadcast
- 同一使用者可有多個連線（多分頁）

## 訊息協議

```go
type Message struct {
    Type    string      `json:"type"`
    Payload interface{} `json:"payload"`
}
```

已定義的 type：
- `import_progress` — 逐檔匯入進度
- `import_complete` — 匯入完成（含最終 ImportJob）
- `import_error` — 匯入致命錯誤
- `notification` — 通用通知
- `ping` — 心跳

## Notifier Interface

跨層依賴透過 `Notifier` interface 解耦，定義在 `internal/websocket/hub.go`：

```go
type Notifier interface {
    SendToUser(userID string, msg *Message)
    Broadcast(msg *Message)
}
```

Service 層（如 `ImportService`）依賴此 interface，不直接依賴 `Hub` struct。

## 前端重連策略

- 使用 exponential backoff：初始 1s，每次 ×2，上限 30s
- 重連次數上限 20 次，超過停止重連
- 用 `useRef` 追蹤重連次數，避免 re-render 導致計數重置
- 心跳間隔 50s，保持連線活躍

## `onclose` 的副作用要綁 socket 世代，不要用跨世代共用布林

`onclose` 是非同步事件：它觸發時，當初那條 socket 可能早已不是「目前這條」。因此 `onclose` 裡**所有**動作（reset `isConnected`、`cleanup()` 清計時器、`scheduleReconnect()`）都必須先比對 socket 實例身分，不能用單一個跨世代共用的布林 flag 來判斷「這次關閉是不是刻意的」。

反例：用一個共用的 `intentionalCloseRef` 布林。token 快速變更（`tok1 → tok2`，兩者皆 truthy）時，順序是「effect cleanup 設 flag=true 並關掉舊 socket → effect body 設 flag=false 並 `connect()` 建新 socket → 稍後舊 socket 的 `onclose` 才非同步觸發」。舊 socket 的 `onclose` 讀到的 flag 已被新世代覆寫成 false，於是它誤判成非預期斷線：不只多開一條重連，還會用共用的 `cleanup()` 清掉**新** socket 剛裝好的 heartbeat、把 live 連線的 `isConnected` 壓回 false。

正解是用 `wsRef.current` 與閉包捕獲的 `ws` 做三向判斷（每個「刻意關閉」路徑都已把 `wsRef.current` 設成 null 或換成新 socket，所以實例身分足以分辨世代）：

```tsx
ws.onclose = () => {
  // wsRef.current === ws    → 這條 live socket 真的掉了 → reset + 重連
  // wsRef.current === null  → 刻意拆除且無後繼（logout / unmount）→ reset，不重連
  // wsRef.current === 其他   → 已被新世代取代（token 變更）→ 新 socket 自己管狀態，這裡什麼都別做
  if (wsRef.current !== null && wsRef.current !== ws) return
  setIsConnected(false)
  cleanup()
  if (wsRef.current === ws) scheduleReconnect()
}
```

注意 `setIsConnected` 只能放在非同步的 `onclose` 裡，不能搬到 `disconnect()`：`disconnect()` 會被 effect 同步呼叫，`react-hooks` 的 `set-state-in-effect` 規則會擋（同步在 effect 內 setState 觸發 cascading render）。真正的網路斷線（`wsRef.current === ws`）仍照常走 backoff 重連。
