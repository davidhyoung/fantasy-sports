import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getLeagueFeed, listLeagueThreads, getLeagueThread,
  createLeagueThread, createLeagueReply, markThreadRead, pinLeagueThread,
  editLeaguePost, deleteLeaguePost, toggleLeaguePostReaction, voteLeaguePoll,
  type ThreadFilter, type CreatePostRequest,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

/** Invalidates every message-board query for a league — used after any
 *  write, since a post's reply count/last-activity/unread state can shift
 *  the feed, the thread list, and the thread itself all at once. */
function invalidateBoard(qc: ReturnType<typeof useQueryClient>, leagueId: number) {
  qc.invalidateQueries({ queryKey: ['league', leagueId, 'feed'] })
  qc.invalidateQueries({ queryKey: ['league', leagueId, 'threads'] })
  qc.invalidateQueries({ queryKey: ['league', leagueId, 'thread'] })
}

export function useLeagueFeed(leagueId: number, active: boolean) {
  return useQuery({
    queryKey: keys.leagueFeed(leagueId),
    queryFn: () => getLeagueFeed(leagueId),
    enabled: active,
  })
}

export function useLeagueThreads(leagueId: number, filter: ThreadFilter, active: boolean) {
  return useQuery({
    queryKey: keys.leagueThreads(leagueId, filter),
    queryFn: () => listLeagueThreads(leagueId, filter),
    enabled: active,
  })
}

export function useLeagueThread(leagueId: number, threadId: number | null) {
  return useQuery({
    queryKey: keys.leagueThread(leagueId, threadId ?? 0),
    queryFn: () => getLeagueThread(leagueId, threadId!),
    enabled: threadId != null,
  })
}

export function useCreateThread(leagueId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreatePostRequest) => createLeagueThread(leagueId, data),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}

export function useCreateReply(leagueId: number, threadId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreatePostRequest) => createLeagueReply(leagueId, threadId, data),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}

export function useMarkThreadRead(leagueId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (threadId: number) => markThreadRead(leagueId, threadId),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}

export function usePinThread(leagueId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ threadId, pinned }: { threadId: number; pinned: boolean }) =>
      pinLeagueThread(leagueId, threadId, pinned),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}

export function useEditPost(leagueId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ postId, body }: { postId: number; body: string }) => editLeaguePost(leagueId, postId, body),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}

export function useDeletePost(leagueId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (postId: number) => deleteLeaguePost(leagueId, postId),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}

export function useToggleReaction(leagueId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (postId: number) => toggleLeaguePostReaction(leagueId, postId),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}

export function useVotePoll(leagueId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ postId, optionId }: { postId: number; optionId: number }) => voteLeaguePoll(leagueId, postId, optionId),
    onSuccess: () => invalidateBoard(qc, leagueId),
  })
}
