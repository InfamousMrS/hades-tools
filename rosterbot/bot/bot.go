package bot

import (
	"../config"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/rapidloop/skv"
	"strings"
	"time"
)

const CleanupSeconds int = 10
const RosterDB string = "./wsroster.db"

var (
	BotID      string
	BotMention string
	goBot      *discordgo.Session

	ErrPlayerAlreadyAdded   = errors.New("Player already added.")
	ErrPlayerAlreadyRemoved = errors.New("Player already removed.")
	ErrCantLoadRoster       = errors.New("Cant load roster")

	RoleSoldier    string = "soldier"
	RoleSubcommand string = "subcommand"
	RoleCommand    string = "command"
	RoleUnknown    string = "?"
)

var helpMessage string = `Roster Bot Commands:
To enter a command, just @Mention me along with the command. No ! required!

WS Roles:
soldier => follows orders
subcommand => not in charge, but helps with command
command => in charge of the mission
? => No role has been set

Options
join [role] => Join the WS Roster (optionally provide a role)
add @Player [role] => Add one or more players (optionally provide a role)
maybe => Join the WS Group as a 'maybe' 
addmaybe @Player => Add maybes
leave => Leave the WS Group
remove @Player => Remove the tagged player(s) from the WS group.
role [@Player] role => Assign the given role to the player(s)
list => Print a list of players on the roster.
help => Print this message.
`

func Start() {
	goBot, err := discordgo.New("Bot " + config.Token)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	u, err := goBot.User("@me")

	if err != nil {
		fmt.Println(err.Error())
	}

	BotID = u.ID
	BotMention = u.Mention()

	goBot.AddHandler(messageHandler)
	err = goBot.Open()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Println("Roster Bot is running!")
	return
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == BotID {
		return
	}

	if !strings.Contains(m.Content, BotMention) {
		return
	}

	command := strings.ToLower(strings.TrimSpace(strings.Replace(m.Content, BotMention, " ", -1)))
	commandFields := strings.Fields(command)

	handleCommand(s, m, command, commandFields)
}

func getRoleFromCommand(command string) string {
	if strings.Contains(command, RoleSoldier) {
		return RoleSoldier
	} else if strings.Contains(command, RoleSubcommand) {
		return RoleSubcommand
	} else if strings.Contains(command, RoleCommand) {
		return RoleCommand
	} else {
		return RoleUnknown
	}
}

func handleCommand(s *discordgo.Session, m *discordgo.MessageCreate, command string, commandFields []string) {

	if len(commandFields) == 0 {
		respondToChannel(s, m, invalidCommandMessage(m, command), true)
	} else {
		switch commandFields[0] {
		case "help":
			respondToChannel(s, m, helpMessage, false)
		case "join":
			handleAdd(s, m, m.Author, getRoleFromCommand(command))
		case "maybe":
			handleMaybe(s, m, m.Author)
		case "add":
			for _, player := range m.Mentions {
				handleAdd(s, m, player, getRoleFromCommand(command))
			}
		case "addmaybe":
			for _, player := range m.Mentions {
				handleMaybe(s, m, player)
			}
		case "leave":
			handleRemove(s, m, m.Author)
		case "remove":
			for _, player := range m.Mentions {
				handleRemove(s, m, player)
			}
		case "role":
			for _, player := range m.Mentions {
				handleRole(s, m, player, getRoleFromCommand(command))
			}
		case "list":
			sendListReport(s, m)
		default:
			handleCommand(s, m, command, commandFields[1:])
		}
	}
}

func invalidCommandMessage(m *discordgo.MessageCreate, command string) string {
	return m.Author.Mention() + " - Invalid Command: " + command
}

func respondToChannel(s *discordgo.Session, m *discordgo.MessageCreate, message string, cleanup bool) {
	msg, err := s.ChannelMessageSend(m.ChannelID, message)
	if err == nil {
		if cleanup {
			fmt.Println("cleaning up")
			time.Sleep(10 * time.Second)
			s.ChannelMessageDelete(m.ChannelID, m.ID)
			s.ChannelMessageDelete(msg.ChannelID, msg.ID)
			fmt.Println("done cleaning up")
		}
	}
}

func handleRole(s *discordgo.Session, m *discordgo.MessageCreate, player *discordgo.User, role string) error {
	if player.ID != BotID {
		err := saveRole(player.Username, role)
		if err != nil {
			return err
		}
		respondToChannel(s, m, string("Set player "+player.Username+" to role: "+role+"."), true)
	}
	return nil
}

func handleAdd(s *discordgo.Session, m *discordgo.MessageCreate, player *discordgo.User, role string) error {
	if player.ID != BotID {
		err := doRemoveMaybe(player)
		err = doAdd(player, role)
		err = saveRole(player.Username, role)
		if err != nil {
			return err
		}
		respondToChannel(s, m, string("Added player "+player.Username+" to white star as "+role+"."), true)
	}
	return nil
}

func handleMaybe(s *discordgo.Session, m *discordgo.MessageCreate, player *discordgo.User) error {
	if player.ID != BotID {
		err := doRemove(player)
		err = doMaybe(player)
		if err != nil {
			return err
		}
		respondToChannel(s, m, string("Added player "+player.Username+" to white star as a 'Maybe'"), true)
	}
	return nil
}

func doAdd(player *discordgo.User, role string) error {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		return err
	}
	roster := make(map[string]discordgo.User)
	store.Get("roster", &roster)
	_, exists := roster[player.Username]
	if exists {
		fmt.Println("Not adding user: " + player.Username + ", already in ws roster.")
		return ErrPlayerAlreadyAdded
	} else {
		roster[player.Username] = *player
		store.Put("roster", roster)
		fmt.Println("Added user: " + player.Username + " to the ws roster.")
		return nil
	}
	return nil
}

func doMaybe(player *discordgo.User) error {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		return err
	}
	roster := make(map[string]discordgo.User)
	store.Get("maybe", &roster)
	_, exists := roster[player.Username]
	if exists {
		fmt.Println("Not adding user: " + player.Username + ", already in maybe roster.")
		return ErrPlayerAlreadyAdded
	} else {
		roster[player.Username] = *player
		store.Put("maybe", roster)
		fmt.Println("Added user: " + player.Username + " to the maybe roster.")
		return nil
	}
	return nil
}

func handleRemove(s *discordgo.Session, m *discordgo.MessageCreate, player *discordgo.User) error {
	err := doRemove(player)
	if err != nil {
		err := doRemoveMaybe(player)
		if err != nil {
			return err
		}
	}
	respondToChannel(s, m, string("Removed player "+player.Username+" from WS group."), true)
	return nil
}

func doRemove(player *discordgo.User) error {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		return err
	}
	roster := make(map[string]discordgo.User)
	store.Get("roster", &roster)
	_, exists := roster[player.Username]
	if !exists {
		fmt.Println(player.Username + " is already not on the WS roster.")
		return ErrPlayerAlreadyRemoved
	} else {
		delete(roster, player.Username)
		store.Put("roster", roster)
		fmt.Println("Removed user: " + player.Username + " from the WS roster.")
		return nil
	}
}

func doRemoveMaybe(player *discordgo.User) error {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		return err
	}
	roster := make(map[string]discordgo.User)
	store.Get("maybe", &roster)
	_, exists := roster[player.Username]
	if !exists {
		fmt.Println(player.Username + " is already not on the WS roster.")
		return ErrPlayerAlreadyRemoved
	} else {
		delete(roster, player.Username)
		store.Put("maybe", roster)
		fmt.Println("Removed user: " + player.Username + " from the WS roster.")
		return nil
	}
}

func loadRoster() (map[string]discordgo.User, map[string]discordgo.User, error) {
	store, err := skv.Open(RosterDB)
	if err != nil {
		store.Close()
		return nil, nil, ErrCantLoadRoster
	}
	roster := make(map[string]discordgo.User)
	maybes := make(map[string]discordgo.User)
	store.Get("roster", &roster)
	store.Get("maybe", &maybes)
	store.Close()
	return roster, maybes, nil
}

func sendListReport(s *discordgo.Session, m *discordgo.MessageCreate) {
	roster, maybes, err := loadRoster()
	if err != nil {
		respondToChannel(s, m, "Error, couldn't load the roster.", true)
	}

	output := fmt.Sprintf("```\n(%d)The following players are joining the White Star.\n\n", len(roster))
	for k, _ := range roster {
		roleName, err := getRole(k)
		if err != nil {
			roleName = RoleUnknown
		}
		roleName = fmt.Sprintf("[%s]", roleName)
		output = fmt.Sprintf("%s%-15s%s\n", output, roleName, k)
	}

	output = fmt.Sprintf("%s\n(%d)The following players are 'maybe'.\n\n", output, len(maybes))
	for k, _ := range maybes {
		output = fmt.Sprintf("%s%s\n", output, k)
	}
	output = output + "\n```"

	respondToChannel(s, m, output, false)
}

func saveRole(playerName, role string) error {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		return err
	}
	roles := make(map[string]string)
	store.Get("roles", &roles)
	roles[playerName] = role
	store.Put("roles", roles)
	return nil
}

func getRole(playerName string) (string, error) {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		fmt.Println(err)
		return "", err
	}
	roles := make(map[string]string)
	store.Get("roles", &roles)
	role, exists := roles[playerName]
	if !exists {
		return RoleUnknown, nil
	}
	return role, nil
}

/*
func saveWarpInTime(playerName string, t time.Time) error {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		return err
	}
	warpInTimes := make(map[string]time.Time)
	store.Get("warpInTimes", &warpInTimes)
	warpInTimes[playerName] = t
	store.Put("warpInTimes", warpInTimes)
	return nil
}

func getWarpInTime(playerName string) (time.Time, error) {
	store, err := skv.Open(RosterDB)
	defer store.Close()

	if err != nil {
		return *new(time.Time), err
	}
	warpInTimes := make(map[string]time.Time)
	store.Get("warpInTimes", &warpInTimes)
	inTime, exists := warpInTimes[playerName]
	if !exists {
		return *new(time.Time), nil
	}
	return inTime, nil
}

func warpIn(playerName string) error {
	err := saveWarpInTime(playerName, time.Now())
	if err == nil {
		err = setInside(playerName)
	}
	return err
}

func warpOut(playerName string) error {
	err := setOutside(playerName)
	return err
}

func minutesUntilCooldown(playerName string) int {
	sinceWarpin := minutesSinceWarpin(playerName)
	if sinceWarpin >= 120 {
		return 0
	}
	return 120 - sinceWarpin
}

func minutesSinceWarpin(playerName string) int {
	t, err := getWarpInTime(playerName)
	if err != nil {
		fmt.Printf("Error getting warpin time for %s\n", playerName)
		return -1 // same as if we just warped in
	}
	now := time.Now()
	durationFromWarpin := now.Sub(t)
	return int(durationFromWarpin.Minutes())
}

func rocketsAvailable(playerName string) int {

	status, err := getStatus(playerName)
	if err != nil {
		fmt.Printf("Error getting status for %s\n", playerName)
		return 0
	}

	minutesSinceWarpin := minutesSinceWarpin(playerName)
	onCooldown := minutesSinceWarpin <= 120

	// how many rockets do they have?
	switch status {
	case StatusInside:
		if onCooldown {
			return 1
		} else {
			return 2
		}
	case StatusOutside:
		if onCooldown {
			return 0
		} else {
			return 1
		}
	default:
		fmt.Println("Error determing rockets available so just returning 0")
		return 0
	}
}
*/
