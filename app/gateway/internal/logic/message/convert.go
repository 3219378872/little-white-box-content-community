package message

import (
	"esx/app/gateway/internal/types"
	"esx/app/message/rpc/messageservice"
)

const maxMessagePageSize = 100

func conversationItems(conversations []*messageservice.ConversationInfo) []types.ConversationItem {
	items := make([]types.ConversationItem, 0, len(conversations))
	for _, conversation := range conversations {
		if conversation == nil {
			continue
		}
		items = append(items, types.ConversationItem{
			Id:               conversation.Id,
			TargetUserId:     conversation.TargetUserId,
			TargetUserName:   conversation.TargetUserName,
			TargetUserAvatar: conversation.TargetUserAvatar,
			LastMessage:      conversation.LastMessage,
			LastMessageTime:  conversation.LastMessageTime,
			UnreadCount:      conversation.UnreadCount,
		})
	}
	return items
}

func messageItems(messages []*messageservice.MessageInfo) []types.MessageItem {
	items := make([]types.MessageItem, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		items = append(items, types.MessageItem{
			Id:             message.Id,
			ConversationId: message.ConversationId,
			SenderId:       message.SenderId,
			ReceiverId:     message.ReceiverId,
			Content:        message.Content,
			MsgType:        message.MsgType,
			Status:         message.Status,
			CreatedAt:      message.CreatedAt,
		})
	}
	return items
}
