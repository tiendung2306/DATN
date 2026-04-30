export interface GroupEpochPayload {
  group_id: string
  epoch: number
}

export interface MentionVM {
  user_id: string
  display_name: string
  start: number
  end: number
}

export interface PostVM {
  id: string
  title?: string
  body: string
  author_id: string
  created_at: number
  mentions?: MentionVM[]
}

export interface CommentVM {
  id: string
  post_id: string
  body: string
  author_id: string
  created_at: number
  mentions?: MentionVM[]
  reply_to_comment_id?: string
}
