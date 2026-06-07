import test from 'node:test'
import assert from 'node:assert/strict'

import {
  computeUnreadAnchorUpdate,
  insertSortedUniqueMessage,
  mergeTimelineMessages,
  reconcileCanonicalWithOptimistic,
  reconcileTimelineMessages,
  shouldPerformInitialScroll,
  type UnreadAnchorState,
} from '../src/features/chat/lib/timelineState.ts'
import { isSilentReplayBlocked } from '../src/features/chat/lib/replayBlocked.ts'
import type { ChatMessage } from '../src/stores/useChatStore.ts'

function message(overrides: Partial<ChatMessage> & Pick<ChatMessage, 'id' | 'timestamp'>): ChatMessage {
  return {
    id: overrides.id,
    groupId: overrides.groupId ?? 'group-1',
    sender: overrides.sender ?? 'peer-a',
    content: overrides.content ?? overrides.id,
    timestamp: overrides.timestamp,
    isMine: overrides.isMine ?? false,
    status: overrides.status ?? 'published',
    kind: overrides.kind ?? 'user',
    commentCount: overrides.commentCount,
    localEchoToken: overrides.localEchoToken,
    replayedAt: overrides.replayedAt,
    supersedesMessageId: overrides.supersedesMessageId,
  }
}

function unreadState(anchorId: string | null = null, count = 0): UnreadAnchorState {
  return { anchorId, count }
}

test('insertSortedUniqueMessage places late historical message into timeline order', () => {
  const existing = [
    message({ id: 'm-200', timestamp: 200 }),
    message({ id: 'm-400', timestamp: 400 }),
  ]

  const result = insertSortedUniqueMessage(existing, message({ id: 'm-300', timestamp: 300 }))

  assert.deepEqual(
    result.map((item) => item.id),
    ['m-200', 'm-300', 'm-400'],
  )
})

test('reconcileTimelineMessages hides original only when replacement is present', () => {
  const result = reconcileTimelineMessages([
    message({ id: 'm-old', timestamp: 100, replayedAt: 200 }),
    message({ id: 'm-new', timestamp: 200, supersedesMessageId: 'm-old' }),
  ])

  assert.deepEqual(result.map((item) => item.id), ['m-new'])
})

test('reconcileTimelineMessages keeps original when replacement has not arrived yet', () => {
  const result = reconcileTimelineMessages([
    message({ id: 'm-old', timestamp: 100, replayedAt: 200 }),
  ])

  assert.deepEqual(result.map((item) => item.id), ['m-old'])
})

test('mergeTimelineMessages applies the same replay reconciliation across pagination merges', () => {
  const result = mergeTimelineMessages(
    [message({ id: 'm-old', timestamp: 100, replayedAt: 200 })],
    [message({ id: 'm-new', timestamp: 200, supersedesMessageId: 'm-old' })],
  )

  assert.deepEqual(result.map((item) => item.id), ['m-new'])
})

test('reconcileCanonicalWithOptimistic replaces local optimistic echo with canonical message', () => {
  const result = reconcileCanonicalWithOptimistic(
    [
      message({ id: 'm-100', timestamp: 100 }),
      message({ id: 'local:1', timestamp: 200, sender: 'me', content: 'hello', isMine: true, status: 'sending' }),
    ],
    message({ id: 'm-200', timestamp: 210, sender: 'me', content: 'hello', isMine: true }),
  )

  assert.deepEqual(result.map((item) => item.id), ['m-100', 'm-200'])
  assert.equal(result.some((item) => item.id.startsWith('local:')), false)
})

test('reconcileCanonicalWithOptimistic prefers exact local echo token over content heuristics', () => {
  const result = reconcileCanonicalWithOptimistic(
    [
      message({ id: 'local:token-a', timestamp: 200, sender: 'me', content: 'same', isMine: true, status: 'published', localEchoToken: 'token-a' }),
      message({ id: 'local:token-b', timestamp: 201, sender: 'me', content: 'same', isMine: true, status: 'published', localEchoToken: 'token-b' }),
    ],
    message({ id: 'm-200', timestamp: 210, sender: 'me', content: 'same', isMine: true, localEchoToken: 'token-b' }),
  )

  assert.deepEqual(result.map((item) => item.id), ['local:token-a', 'm-200'])
})

test('reconcileCanonicalWithOptimistic keeps a single row when the same canonical message is merged again', () => {
  const replaced = reconcileCanonicalWithOptimistic(
    [message({ id: 'local:1', timestamp: 200, sender: 'me', content: 'hello', isMine: true, status: 'sending' })],
    message({ id: 'm-200', timestamp: 210, sender: 'me', content: 'hello', isMine: true }),
  )

  const result = insertSortedUniqueMessage(
    replaced,
    message({ id: 'm-200', timestamp: 210, sender: 'me', content: 'hello', isMine: true }),
  )

  assert.deepEqual(result.map((item) => item.id), ['m-200'])
})

test('reconcileCanonicalWithOptimistic does not match failed optimistic messages', () => {
  const result = reconcileCanonicalWithOptimistic(
    [message({ id: 'local:failed', timestamp: 200, sender: 'me', content: 'hello', isMine: true, status: 'failed' })],
    message({ id: 'm-200', timestamp: 210, sender: 'me', content: 'hello', isMine: true }),
  )

  assert.deepEqual(result.map((item) => item.id), ['local:failed', 'm-200'])
})

test('reconcileCanonicalWithOptimistic reconciles two near-identical optimistic sends independently', () => {
  const afterFirst = reconcileCanonicalWithOptimistic(
    [
    message({ id: 'local:1', timestamp: 200, sender: 'me', content: 'same', isMine: true, status: 'sending' }),
    message({ id: 'local:2', timestamp: 201, sender: 'me', content: 'same', isMine: true, status: 'sending' }),
    ],
    message({ id: 'm-201', timestamp: 220, sender: 'me', content: 'same', isMine: true }),
  )

  const afterSecond = reconcileCanonicalWithOptimistic(
    afterFirst,
    message({ id: 'm-202', timestamp: 221, sender: 'me', content: 'same', isMine: true }),
  )

  assert.deepEqual(afterSecond.map((item) => item.id), ['m-201', 'm-202'])
})

test('reconcileCanonicalWithOptimistic matches even when optimistic sender is stale or empty', () => {
  const result = reconcileCanonicalWithOptimistic(
    [message({ id: 'local:1', timestamp: 200, sender: '', content: 'hello', isMine: true, status: 'sending' })],
    message({ id: 'm-200', timestamp: 210, sender: 'peer-real', content: 'hello', isMine: true }),
  )

  assert.deepEqual(result.map((item) => item.id), ['m-200'])
  assert.equal(result[0]?.sender, 'peer-real')
})

test('reconcileCanonicalWithOptimistic does not replace local optimistic row for remote message with same content', () => {
  const result = reconcileCanonicalWithOptimistic(
    [message({ id: 'local:1', timestamp: 200, sender: '', content: 'hello', isMine: true, status: 'sending' })],
    message({ id: 'm-remote', timestamp: 210, sender: 'peer-b', content: 'hello', isMine: false }),
  )

  assert.deepEqual(result.map((item) => item.id), ['local:1', 'm-remote'])
})

test('reconcileTimelineMessages hides original when multiple replacements canonicalize to the same root', () => {
  const result = reconcileTimelineMessages([
    message({ id: 'm-root', timestamp: 100, replayedAt: 150 }),
    message({ id: 'm-latest', timestamp: 300, supersedesMessageId: 'm-root' }),
  ])

  assert.deepEqual(result.map((item) => item.id), ['m-latest'])
})

test('isSilentReplayBlocked keeps stale late-join events silent for both new and old payload shapes', () => {
  assert.equal(
    isSilentReplayBlocked({ reason: 'stale_epoch_requires_recovery_snapshot', user_visible: false }),
    true,
  )
  assert.equal(
    isSilentReplayBlocked({ reason: 'stale_epoch_requires_recovery_snapshot' }),
    true,
  )
  assert.equal(
    isSilentReplayBlocked({ reason: 'decrypt_failed_or_missing_past_key' }),
    false,
  )
})

test('computeUnreadAnchorUpdate tracks late historical sync while reading older messages', () => {
  const previousMessages = [
    message({ id: 'm-200', timestamp: 200 }),
    message({ id: 'm-400', timestamp: 400 }),
  ]
  const nextMessages = [
    message({ id: 'm-200', timestamp: 200 }),
    message({ id: 'm-300', timestamp: 300, sender: 'peer-b' }),
    message({ id: 'm-400', timestamp: 400 }),
  ]

  const result = computeUnreadAnchorUpdate({
    previousMessages,
    nextMessages,
    current: unreadState(),
    isAtBottom: false,
  })

  assert.deepEqual(result, unreadState('m-300', 1))
})

test('computeUnreadAnchorUpdate ignores manual load-more pagination batches', () => {
  const previousMessages = [
    message({ id: 'm-300', timestamp: 300 }),
    message({ id: 'm-400', timestamp: 400 }),
  ]
  const nextMessages = [
    message({ id: 'm-100', timestamp: 100 }),
    message({ id: 'm-200', timestamp: 200 }),
    message({ id: 'm-300', timestamp: 300 }),
    message({ id: 'm-400', timestamp: 400 }),
  ]

  const result = computeUnreadAnchorUpdate({
    previousMessages,
    nextMessages,
    current: unreadState(),
    isAtBottom: false,
    suppressTracking: true,
  })

  assert.deepEqual(result, unreadState())
})

test('computeUnreadAnchorUpdate does not mark own reconciled message as unread', () => {
  const previousMessages = [
    message({ id: 'm-100', timestamp: 100 }),
    message({ id: 'local:pending', timestamp: 390, isMine: true, sender: 'me' }),
  ]
  const nextMessages = [
    message({ id: 'm-100', timestamp: 100 }),
    message({ id: 'm-450', timestamp: 450, isMine: true, sender: 'me' }),
  ]

  const result = computeUnreadAnchorUpdate({
    previousMessages,
    nextMessages,
    current: unreadState(),
    isAtBottom: false,
  })

  assert.deepEqual(result, unreadState())
})

test('shouldPerformInitialScroll only returns true for first room load completion', () => {
  assert.equal(shouldPerformInitialScroll('group-1', 'group-1', false), true)
  assert.equal(shouldPerformInitialScroll('group-1', 'group-1', true), false)
  assert.equal(shouldPerformInitialScroll('group-1', null, false), false)
  assert.equal(shouldPerformInitialScroll(null, null, false), false)
})
