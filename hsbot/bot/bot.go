package bot

import (
	"../config"
	"./botcommand"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/rapidloop/skv"
	"strings"
	"time"
)

const CleanupSeconds int = 10
const StatusInside string = "Inside"
const StatusOutside string = "Outside"
const StatusInvalid string = "Invalid"

var (
	BotID      string
	BotMention string
	goBot      *discordgo.Session

	ErrPlayerAlreadyAdded   = errors.New("Player already added.")
	ErrPlayerAlreadyRemoved = errors.New("Player already removed.")
	ErrCantLoadRoster       = errors.New("Cant load roster")
)

var helpMessage string = `Rocket Bot Commands:
To enter a command, just @Mention me along with the command. No ! required!

Options
join => Join the rocket brigade.
leave => Leave the rocket brigade.
add @Player => Add the tagged player(s) to the rocket brigade. 
remove @Player => Remove the tagged player(s) to the rocket brigade.
warp in => Tell rocket bot that your ship has warped into the White Star.
warp out => Tell rocket bot that your ship has warped out of the White Star.
report => Print an availability report for the rocket brigade.
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

	fmt.Println("Bot is running!")
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

func handleCommand(s *discordgo.Session, m *discordgo.MessageCreate, command string, commandFields []string) {

	if len(commandFields) == 0 {
		respondToChannel(s, m, invalidCommandMessage(m, command), true)
	} else {
		switch commandFields[0] {
		case "help":
			respondToChannel(s, m, helpMessage, false)
		case "join":
			handleAdd(s, m, m.Author)
		case "add":
			for _, player := range m.Mentions {
				handleAdd(s, m, player)
			}
		case "leave":
			handleRemove(s, m, m.Author)
		case "remove":
			for _, player := range m.Mentions {
				handleRemove(s, m, player)
			}
		case "warp":
			handleCommand(s, m, command, commandFields[1:])
		case "warpin", "in", "warp-in":
			handleWarpIn(s, m)
		case "warpout", "out", "warp-out":
			err := warpOut(m.Author.Username)
			if err == nil {
				respondToChannel(s, m, m.Author.Username+" warped out. Status is Outside.", true)
			} else {
				respondToChannel(s, m, "Error: couldn't record warp out for "+m.Author.Username, true)
			}
		case "list":
			sendListReport(s, m)
		case "report":
			sendReport(s, m)
		case "fire":
			return // order members to fire.
		case "cooldown":
			return // update the cooldown time
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
			time.Sleep(5 * time.Second)
			s.ChannelMessageDelete(m.ChannelID, m.ID)
			s.ChannelMessageDelete(msg.ChannelID, msg.ID)
			fmt.Println("done cleaning up")
		}
	}
}

func handleWarpIn(s *discordgo.Session, m *discordgo.MessageCreate) {
	err := warpIn(m.Author.Username)
	if err == nil {
		respondToChannel(s, m, m.Author.Username+" warped in. Cooldown set as 2hrs. Status is Inside.", true)
	} else {
		respondToChannel(s, m, "Error: couldn't record warp in for "+m.Author.Username, true)
	}
}

func handleWarpOut(s *discordgo.Session, m *discordgo.MessageCreate) {
	err := warpOut(m.Author.Username)
	if err == nil {
		respondToChannel(s, m, m.Author.Username+" warped out. Status is Outside.", true)
	} else {
		respondToChannel(s, m, "Error: couldn't record warp out for "+m.Author.Username, true)
	}
}

func handleAdd(s *discordgo.Session, m *discordgo.MessageCreate, player *discordgo.User) error {
	if player.ID != BotID {
		err := doAdd(player)
		if err != nil {
			return err
		}
		err = setOutside(player.Username)
		if err != nil {
			return err
		}
		respondToChannel(s, m, string("Added player "+player.Username+" to rocket group."), true)
	}
	return nil
}

func doAdd(player *discordgo.User) error {
	store, err := skv.Open("../rocket.db")
	defer store.Close()

	if err != nil {
		return err
	}
	roster := make(map[string]discordgo.User)
	store.Get("roster", &roster)
	_, exists := roster[player.Username]
	if exists {
		fmt.Println("Not adding user: " + player.Username + ", already in rocket roster.")
		return ErrPlayerAlreadyAdded
	} else {
		roster[player.Username] = *player
		store.Put("roster", roster)
		fmt.Println("Added user: " + player.Username + " to the rocket roster.")
		return nil
	}
	return nil
}

func handleRemove(s *discordgo.Session, m *discordgo.MessageCreate, player *discordgo.User) error {
	err := doRemove(player)
	if err != nil {
		return err
	}
	respondToChannel(s, m, string("Removed player "+player.Username+" from rocket group."), true)
	return nil
}

func doRemove(player *discordgo.User) error {
	store, err := skv.Open("../rocket.db")
	defer store.Close()

	if err != nil {
		return err
	}
	roster := make(map[string]discordgo.User)
	store.Get("roster", &roster)
	_, exists := roster[player.Username]
	if !exists {
		fmt.Println(player.Username + " is already not on the rocket roster.")
		return ErrPlayerAlreadyRemoved
	} else {
		delete(roster, player.Username)
		store.Put("roster", roster)
		fmt.Println("Removed user: " + player.Username + " from the rocket roster.")
		return nil
	}
}

func loadRoster() (map[string]discordgo.User, error) {
	store, err := skv.Open("../rocket.db")
	if err != nil {
		store.Close()
		return nil, ErrCantLoadRoster
	}
	roster := make(map[string]discordgo.User)
	store.Get("roster", &roster)
	store.Close()
	return roster, nil
}

func sendListReport(s *discordgo.Session, m *discordgo.MessageCreate) {
	roster, err := loadRoster()
	if err != nil {
		respondToChannel(s, m, "Error, couldn't load the roster.", true)
	}

	output := "The following players are assigned to rocket duty.\n"
	for k, _ := range roster {
		output = fmt.Sprintf("%s%s\n", output, k)
	}

	respondToChannel(s, m, output, false)
}

func sendReport(s *discordgo.Session, m *discordgo.MessageCreate) {

	roster, err := loadRoster()
	if err != nil {
		respondToChannel(s, m, "Error, couldn't load the roster.", true)
	}

	output := "```\nThe following players are in the group:\n"
	for k, v := range roster {
		status, _ := getStatus(v.Username)
		statusOutput := fmt.Sprintf("Status[%s]", status)
		cooldownMinutes := minutesUntilCooldown(v.Username)
		cooldownOutput := fmt.Sprintf("Cooldown[%d min]", cooldownMinutes)
		rockets := rocketsAvailable(v.Username)
		rocketsOutput := fmt.Sprintf("RocketsReady[%d]", rockets)
		output = fmt.Sprintf("%s%-17s%-17s%-18s%-16s\n", output, k, statusOutput, cooldownOutput, rocketsOutput)
	}
	output = output + "```"

	respondToChannel(s, m, output, false)
}

func setInside(playerName string) error {
	return saveStatus(playerName, StatusInside)
}

func setOutside(playerName string) error {
	return saveStatus(playerName, StatusOutside)
}

func saveStatus(playerName, status string) error {
	store, err := skv.Open("../rocket.db")
	defer store.Close()

	if err != nil {
		return err
	}
	statuses := make(map[string]string)
	store.Get("statuses", &statuses)
	statuses[playerName] = status
	store.Put("statuses", statuses)
	return nil
}

func getStatus(playerName string) (string, error) {
	store, err := skv.Open("../rocket.db")
	defer store.Close()

	if err != nil {
		fmt.Println(err)
		return "", err
	}
	statuses := make(map[string]string)
	store.Get("statuses", &statuses)
	status, exists := statuses[playerName]
	if !exists {
		return StatusInvalid, nil
	}
	return status, nil
}

func saveWarpInTime(playerName string, t time.Time) error {
	store, err := skv.Open("../rocket.db")
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
	store, err := skv.Open("../rocket.db")
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
