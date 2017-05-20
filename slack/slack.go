// Package slack implements Slack handlers for github.com/go-chat-bot/bot
package slack

import (
	"errors"
	"fmt"

	"github.com/nlopes/slack"
	"github.com/xcsrz/go-chat-bot"
)

var (
	rtm      *slack.RTM
	api      *slack.Client
	teaminfo *slack.TeamInfo

	channelList = map[string]slack.Channel{}
	userList    = map[string]slack.User{}
	groupList   = map[string]slack.Group{}
	params      = slack.PostMessageParameters{AsUser: true}
	botUserID   = ""
	Initialized = make(chan struct{})
	initialized = false
)

func responseHandler(target string, message string, sender *bot.User) {
	api.PostMessage(target, message, params)
}

func PostMessage(room, message string) error {
	code, found := channelFromName(room)
	if !found {
		return errors.New("Could not identify room")
	}
	responseHandler(code, message, nil)
	return nil
}

func channelFromName(name string) (code string, found bool) {
	for _, channel := range channelList {
		if channel.Name == name {
			return channel.ID, true
		}
	}
	for _, group := range groupList {
		if group.Name == name {
			return group.ID, true
		}
	}
	for _, user := range userList {
		if user.Name == name {
			return user.ID, true
		}
	}
	return "", false
}

// Extracts user information from slack API
func extractUser(event *slack.MessageEvent) *bot.User {
	var isBot bool
	var userID string
	if len(event.User) == 0 {
		userID = event.BotID
		isBot = true
	} else {
		userID = event.User
		isBot = false
	}
	slackUser, err := api.GetUserInfo(userID)
	if err != nil {
		fmt.Printf("Error retrieving slack user: %s\n", err)
		return &bot.User{
			ID:    userID,
			IsBot: isBot}
	}
	return &bot.User{
		ID:       userID,
		Nick:     slackUser.Name,
		RealName: slackUser.Profile.RealName,
		IsBot:    isBot}
}

func extractText(event *slack.MessageEvent) string {
	text := ""
	if len(event.Text) != 0 {
		text = event.Text
	} else {
		attachments := event.Attachments
		if len(attachments) > 0 {
			text = attachments[0].Fallback
		}
	}
	return text
}

func readBotInfo(api *slack.Client) {
	info, err := api.AuthTest()
	if err != nil {
		fmt.Printf("Error calling AuthTest: %s\n", err)
		return
	}
	botUserID = info.UserID
}

func readChannelData(api *slack.Client) {
	channels, err := api.GetChannels(true)
	if err != nil {
		fmt.Printf("Error getting Channels: %s\n", err)
		return
	}
	for _, channel := range channels {
		channelList[channel.ID] = channel
	}

	groups, err := api.GetGroups(true)
	if err != nil {
		fmt.Printf("Error getting Groups: %s\n", err)
		return
	}
	for _, group := range groups {
		groupList[group.ID] = group
	}

	users, err := api.GetUsers()
	if err != nil {
		fmt.Printf("Error getting User Channels: %s\n", err)
		return
	}
	for _, user := range users {
		userList[user.ID] = user
	}
	if !initialized {
		close(Initialized)
		initialized = true
	}
}

func ownMessage(UserID string) bool {
	return botUserID == UserID
}

// Run connects to slack RTM API using the provided token
func Run(token string) {
	api = slack.New(token)
	rtm = api.NewRTM()
	teaminfo, _ = api.GetTeamInfo()

	b := bot.New(&bot.Handlers{
		Response: responseHandler,
	})
	b.Disable([]string{"url"})

	go rtm.ManageConnection()

Loop:
	for {
		select {
		case msg := <-rtm.IncomingEvents:
			switch ev := msg.Data.(type) {
			case *slack.HelloEvent:
				readBotInfo(api)
				readChannelData(api)
			case *slack.ChannelCreatedEvent:
				readChannelData(api)
			case *slack.ChannelRenameEvent:
				readChannelData(api)

			case *slack.MessageEvent:
				if !ev.Hidden && !ownMessage(ev.User) {
					C := channelList[ev.Channel]
					var channel = ev.Channel
					if C.IsChannel {
						channel = fmt.Sprintf("#%s", C.Name)
					}
					b.MessageReceived(
						&bot.ChannelData{
							Protocol:  "slack",
							Server:    teaminfo.Domain,
							Channel:   channel,
							IsPrivate: !C.IsChannel,
						},
						extractText(ev),
						extractUser(ev))
				}

			case *slack.RTMError:
				fmt.Printf("Error: %s\n", ev.Error())

			case *slack.InvalidAuthEvent:
				fmt.Printf("Invalid credentials")
				break Loop
			}
		}
	}
}
