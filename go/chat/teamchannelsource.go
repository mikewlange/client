package chat

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru"
	"github.com/keybase/client/go/chat/globals"
	"github.com/keybase/client/go/chat/types"
	"github.com/keybase/client/go/chat/utils"
	"github.com/keybase/client/go/protocol/chat1"
	"github.com/keybase/client/go/protocol/gregor1"
)

type teamChannelCacheItem struct {
	dat   []chat1.ChannelNameMention
	entry time.Time
}

type CachingTeamChannelSource struct {
	globals.Contextified
	utils.DebugLabeler
	sync.Mutex

	offline bool
	cache   *lru.Cache
	ri      func() chat1.RemoteInterface
}

var _ types.TeamChannelSource = (*CachingTeamChannelSource)(nil)

func NewCachingTeamChannelSource(g *globals.Context, ri func() chat1.RemoteInterface) *CachingTeamChannelSource {
	c, err := lru.New(100)
	if err != nil {
		panic(err)
	}
	return &CachingTeamChannelSource{
		Contextified: globals.NewContextified(g),
		DebugLabeler: utils.NewDebugLabeler(g.GetLog(), "CachingTeamChannelSource", false),
		ri:           ri,
		cache:        c,
	}
}

func (c *CachingTeamChannelSource) key(teamID chat1.TLFID, topicType chat1.TopicType) string {
	return fmt.Sprintf("tid:%v,tt:%v", teamID.String(), int(topicType))
}

func (c *CachingTeamChannelSource) fetchFromCache(ctx context.Context,
	teamID chat1.TLFID, topicType chat1.TopicType) (res []chat1.ChannelNameMention, ok bool) {
	val, ok := c.cache.Get(c.key(teamID, topicType))
	if !ok {
		return res, false
	}
	var item teamChannelCacheItem
	if item, ok = val.(teamChannelCacheItem); !ok {
		return nil, false
	}
	// Check to see if the entry is stale, and get it out of there if so
	if time.Now().Sub(item.entry) > time.Hour {
		c.invalidate(ctx, teamID)
		return nil, false
	}
	return item.dat, true
}

func (c *CachingTeamChannelSource) writeToCache(ctx context.Context,
	teamID chat1.TLFID, topicType chat1.TopicType, names []chat1.ChannelNameMention) {
	c.cache.Add(c.key(teamID, topicType), teamChannelCacheItem{
		dat:   names,
		entry: time.Now(),
	})
}

func (c *CachingTeamChannelSource) invalidate(ctx context.Context, teamID chat1.TLFID) {
	for _, topicType := range chat1.TopicTypeMap {
		c.cache.Remove(c.key(teamID, topicType))
	}
}

func (c *CachingTeamChannelSource) GetChannelsFull(ctx context.Context, uid gregor1.UID, teamID chat1.TLFID,
	topicType chat1.TopicType) (res []chat1.ConversationLocal, rl []chat1.RateLimit, err error) {
	var convs []chat1.Conversation
	tlfRes, err := c.ri().GetTLFConversations(ctx, chat1.GetTLFConversationsArg{
		TlfID:            teamID,
		TopicType:        topicType,
		SummarizeMaxMsgs: false,
	})
	if err != nil {
		return res, rl, err
	}
	if tlfRes.RateLimit != nil {
		rl = append(rl, *tlfRes.RateLimit)
	}
	convs = tlfRes.Conversations

	// Localize the conversations
	res, err = NewBlockingLocalizer(c.G()).Localize(ctx, uid, types.Inbox{
		ConvsUnverified: utils.RemoteConvs(convs),
	})
	if err != nil {
		c.Debug(ctx, "GetChannelsFull: failed to localize conversations: %s", err.Error())
		return res, rl, err
	}
	sort.Sort(utils.ConvLocalByTopicName(res))
	rl = utils.AggRateLimits(rl)
	return res, rl, nil
}

func (c *CachingTeamChannelSource) GetChannelsTopicName(ctx context.Context, uid gregor1.UID,
	teamID chat1.TLFID, topicType chat1.TopicType) (res []chat1.ChannelNameMention, rl []chat1.RateLimit, err error) {

	var ok bool
	if res, ok = c.fetchFromCache(ctx, teamID, topicType); ok {
		c.Debug(ctx, "GetChannelsTopicName: cache hit")
		return res, rl, nil
	}

	var convs []chat1.Conversation
	tlfRes, err := c.ri().GetTLFConversations(ctx, chat1.GetTLFConversationsArg{
		TlfID:            teamID,
		TopicType:        topicType,
		SummarizeMaxMsgs: false,
	})
	if err != nil {
		c.Debug(ctx, "GetChannelsTopicName: failed to get TLF convos: %s", err)
		return res, rl, err
	}
	if tlfRes.RateLimit != nil {
		rl = append(rl, *tlfRes.RateLimit)
	}
	convs = tlfRes.Conversations

	getMetadataMsg := func(conv chat1.Conversation) (chat1.MessageBoxed, bool) {
		for _, msg := range conv.MaxMsgs {
			if msg.GetMessageType() == chat1.MessageType_METADATA {
				return msg, true
			}
		}
		return chat1.MessageBoxed{}, false
	}

	// Find metadata messages in this result and unbox them
	for _, conv := range convs {
		msg, ok := getMetadataMsg(conv)
		if ok {
			unboxeds, err := c.G().ConvSource.GetMessagesWithRemotes(ctx, conv, uid, []chat1.MessageBoxed{msg})
			if err != nil {
				c.Debug(ctx, "GetChannelsTopicName: failed to unbox metadata message for: convID: %s err: %s",
					conv.GetConvID(), err)
				continue
			}
			if len(unboxeds) != 1 {
				c.Debug(ctx, "GetChannelsTopicName: empty result: convID: %s", conv.GetConvID())
				continue
			}
			unboxed := unboxeds[0]
			if !unboxed.IsValid() {
				c.Debug(ctx, "GetChannelsTopicName: metadata message invalid: convID, %s",
					conv.GetConvID())
				continue
			}
			body := unboxed.Valid().MessageBody
			typ, err := body.MessageType()
			if err != nil {
				c.Debug(ctx, "GetChannelsTopicName: error getting message type: convID, %s",
					conv.GetConvID(), err)
				continue
			}
			if typ != chat1.MessageType_METADATA {
				c.Debug(ctx, "GetChannelsTopicName: message not a real metadata message: convID, %s msgID: %d",
					conv.GetConvID(), unboxed.GetMessageID())
				continue
			}

			res = append(res, chat1.ChannelNameMention{
				ConvID:    conv.GetConvID(),
				TopicName: body.Metadata().ConversationTitle,
			})
		}
	}

	c.writeToCache(ctx, teamID, topicType, res)
	return res, rl, nil
}

func (c *CachingTeamChannelSource) GetChannelTopicName(ctx context.Context, uid gregor1.UID, tlfID chat1.TLFID,
	topicType chat1.TopicType, convID chat1.ConversationID) (topicName string, rl []chat1.RateLimit, err error) {
	convs, rl, err := c.GetChannelsTopicName(ctx, uid, tlfID, topicType)
	if err != nil {
		return topicName, rl, err
	}
	if len(convs) == 0 {
		return topicName, rl, fmt.Errorf("no convs found")
	}
	for _, conv := range convs {
		if conv.ConvID.Eq(convID) {
			return conv.TopicName, rl, nil
		}
	}
	return topicName, rl, fmt.Errorf("no convs found with conv ID")
}

func (c *CachingTeamChannelSource) ChannelsChanged(ctx context.Context, teamID chat1.TLFID) {
	if len(teamID) == 0 {
		// Clear everything with blank TLF ID
		c.Debug(ctx, "ChannelsChanged: blank TLFID, dropping entire cache")
		c.cache.Purge()
	} else {
		c.invalidate(ctx, teamID)
	}
}

func (c *CachingTeamChannelSource) IsOffline(ctx context.Context) bool {
	c.Lock()
	defer c.Unlock()
	return c.offline
}

func (c *CachingTeamChannelSource) Connected(ctx context.Context) {
	c.Lock()
	defer c.Unlock()
	c.Debug(ctx, "Connected: dropping cache")
	c.cache.Purge()
	c.offline = false
}

func (c *CachingTeamChannelSource) Disconnected(ctx context.Context) {
	c.Lock()
	defer c.Unlock()
	c.offline = true
}
