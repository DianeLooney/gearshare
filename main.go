package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var wowAPIKey string

func init() {
	wowAPIKey = ""

	data, err := ioutil.ReadFile("data/data.json")
	if err != nil {
		log.Fatalf("Unable to read data.json\n")
	}
	err = json.Unmarshal(data, &wowData)
	if err != nil {
		log.Fatalf("Malformed json found in data.json: %v\n", err)
	}
	templates = make(map[string]*template.Template)
	templates["playergear"], err = template.New("playergear").ParseFiles("templates/playergear.tmpl.html")
	if err != nil {
		log.Fatalf("Unable to parse template at %v: %v\n", "templates/playergear.html.tmpl", err)
	}
}

var rgxSimcArtifact = regexp.MustCompile(`^\s*artifact=([0-9:]*)$`)
var rgxSimcCrucible = regexp.MustCompile(`^\s*crucible=([0-9:/]*)$`)
var rgxSimcProfessions = regexp.MustCompile(`^\s*professions=(.*)$`)
var rgxSimcRole = regexp.MustCompile(`^\s*role=(.*)$`)
var rgxSimcRace = regexp.MustCompile(`^\s*race=(.*)$`)
var rgxSimcRegion = regexp.MustCompile(`^\s*region=(.*)$`)
var rgxSimcSpec = regexp.MustCompile(`^\s*spec=(.*)$`)
var rgxSimcServer = regexp.MustCompile(`^\s*server=(.*)$`)
var rgxSimcClassName = regexp.MustCompile(`^\s*(druid|demonhunter)=(".*"|.*)$`)
var rgxSimcLevel = regexp.MustCompile(`^\s*level=(.*)$`)
var rgxSimcItem = regexp.MustCompile(`^\s*(head|neck|shoulder|back|chest|wrist|hands|waist|legs|feet|finger1|finger2|trinket1|trinket2|main_hand|off_hand)=(.*)$`)
var rgxSimcItemCommented = regexp.MustCompile(`^\s*[#]+\s*(head|neck|shoulder|back|chest|wrist|hands|waist|legs|feet|finger1|finger2|trinket1|trinket2|main_hand|off_hand)=(.*)$`)
var rgxSimcItemArg = regexp.MustCompile(`^([a-z_A-Z]*)=([^,]*)`)
var rgxSimcComment = regexp.MustCompile(`^\s*[#](.*)$`)
var rgxSimcTalents = regexp.MustCompile(`^\s*talents=([0123])*$`)

func main() {
	d, err := ioutil.ReadFile("input/savedyabear-resto.simc")
	if err != nil {
		log.Fatalf("Savedyabear not found!")
	}
	s := string(d)
	c := parseCharacterInfo(s)
	buf := &bytes.Buffer{}
	err = templates["playergear"].ExecuteTemplate(buf, "T", c)
	if err != nil {
		log.Fatalf("Unable to execute template: %v\n", err)
	}
	err = ioutil.WriteFile("output/savedyabear-resto.html", buf.Bytes(), 0666)
	if err != nil {
		log.Fatalf("Unable to write to file: %v\n", err)
	}
}

var templates map[string]*template.Template

type wowDataStruct struct {
	TierBonusIDs map[int]string
	StatIDs      map[int]string
	Primary      []string
	Secondary    []string
	Tertiary     []string
	I18N         map[string]statsLocalization
}
type statsLocalization map[string]string

var wowData wowDataStruct

func localize(s, i string) string {
	loc, ok := wowData.I18N[i]
	if !ok {
		return fmt.Sprintf("[%v]", s)
	}
	str, ok := loc[s]
	if !ok {
		return fmt.Sprintf("[%v]", s)
	}
	return str
}

type CharacterInfo struct {
	Class       string
	Name        string
	Level       int
	Race        string
	Region      string
	Server      string
	Spec        string
	Professions string
	Talents     []int
	Artifact    []struct {
		a int
		b int
	}
	Crucible []struct {
		a int
		b int
	}
	Items []ItemInfo
}

func (c CharacterInfo) Slots() []SlotSummary {
	x := []string{"head",
		"neck",
		"shoulder",
		"back",
		"chest",
		"wrist",
		"hands",
		"waist",
		"legs",
		"feet",
		"finger",
		"trinket"}
	retval := make([]SlotSummary, 0)
	for _, v := range x {
		y := make([]ItemInfo, 0)
		for _, t := range c.Items {
			if t.Slot == v ||
				(t.Slot == "trinket1" && v == "trinket") ||
				(t.Slot == "trinket2" && v == "trinket") ||
				(t.Slot == "finger1" && v == "finger") ||
				(t.Slot == "finger2" && v == "finger") {
				y = append(y, t)
			}
		}
		sort.Slice(y, func(i, j int) bool {
			if y[i].Equipped {
				return true
			}
			if y[j].Equipped {
				return false
			}
			return y[i].Level > y[j].Level
		})
		retval = append(retval, SlotSummary{v, y})
	}

	return retval
}
func (c CharacterInfo) FilterBySlot(n string) []ItemInfo {
	ret := make([]ItemInfo, 0)
	for _, x := range c.Items {
		if x.Slot == n ||
			(x.Slot == "trinket1" && n == "trinket2") ||
			(x.Slot == "trinket2" && n == "trinket1") ||
			(x.Slot == "finger1" && n == "finger2") ||
			(x.Slot == "finger2" && n == "finger1") {
			ret = append(ret, x)
		}
	}
	return ret
}

type SlotSummary struct {
	Slot  string
	Items []ItemInfo
}
type ItemInfo struct {
	ID       int    `json:"id"`
	Icon     string `json:"icon"`
	BonusIDs []int  `json:"bonusLists"`
	Name     string `json:"name"`
	Slot     string
	Level    int `json:"itemLevel"`
	Set      struct {
		ID int `json:"id"`
	} `json:"itemSet"`
	//Enchant       int        `json:""`
	Sockets struct {
		Sockets []struct{} `json:"sockets"`
	} `json:"socketInfo"`
	Quality  string `json:"context"`
	SlotType int    `json:"inventoryType"`
	Armor    int    `json:"armor"`
	Equipped bool
	Stats    []ItemStat `json:"bonusStats"`
}

func (i ItemInfo) TierBonus() string {
	if i.Set.ID != 0 {
		l, ok := wowData.TierBonusIDs[i.Set.ID]
		if ok {
			return l
		}
		return "T??"
	}
	return ""
}
func (i ItemInfo) SocketCount() string {
	if len(i.Sockets.Sockets) == 0 {
		return ""
	}
	return strconv.Itoa(len(i.Sockets.Sockets))
}

type ItemStat struct {
	Stat   int `json:"stat"`
	Amount int `json:"amount"`
}

func (i ItemInfo) GetStat(n string) string {
	for _, v := range i.Stats {
		k, ok := wowData.StatIDs[v.Stat]
		if !ok {
			continue
		}
		if k == n {
			return strconv.Itoa(v.Amount)
		}
	}
	return ""
}

func parseCharacterInfo(s string) (c *CharacterInfo) {
	c = &CharacterInfo{}
	c.Items = make([]ItemInfo, 0)
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		txt := scanner.Text()
		if len(txt) == 0 {
			continue
		}
		if rgxSimcItemCommented.MatchString(txt) {
			x := rgxSimcItemCommented.FindStringSubmatch(txt)
			itm := ItemInfo{}
			//slot := x[1]
			spl := strings.Split(x[2], ",")

			for _, v := range spl {
				if len(v) == 0 {
					continue
				}

				y := rgxSimcItemArg.FindStringSubmatch(v)
				switch y[1] {
				case "id":
					itm.ID, _ = strconv.Atoi(y[2])
				case "bonus_id":
					z := strings.Split(y[2], "/")
					itm.BonusIDs = make([]int, len(z))
					for i, v := range z {
						itm.BonusIDs[i], _ = strconv.Atoi(v)
					}
				case "enchant_id":
					//itm.Enchant, _ = strconv.Atoi(y[2])
				}

			}
			itm.Slot = x[1]
			c.Items = append(c.Items, itm)
			continue
		}
		if rgxSimcItem.MatchString(txt) {
			x := rgxSimcItem.FindStringSubmatch(txt)
			itm := ItemInfo{}
			//slot := x[1]
			spl := strings.Split(x[2], ",")

			for _, v := range spl {
				if len(v) == 0 {
					continue
				}

				y := rgxSimcItemArg.FindStringSubmatch(v)
				switch y[1] {
				case "id":
					itm.ID, _ = strconv.Atoi(y[2])
				case "bonus_id":
					z := strings.Split(y[2], "/")
					itm.BonusIDs = make([]int, len(z))
					for i, v := range z {
						itm.BonusIDs[i], _ = strconv.Atoi(v)
					}
				case "enchant_id":
					//itm.Enchant, _ = strconv.Atoi(y[2])
				}

			}
			itm.Slot = x[1]
			itm.Equipped = true
			c.Items = append(c.Items, itm)
			continue
		}
		if rgxSimcComment.MatchString(txt) {
			continue
		}
		if rgxSimcClassName.MatchString(txt) {
			x := rgxSimcClassName.FindStringSubmatch(txt)
			c.Class = x[1]
			if x[2][0] == '"' && len(x[2]) > 2 {
				c.Name = x[2][1 : len(x[2])-1]
			} else {
				c.Name = x[2]
			}
			continue
		}
		if rgxSimcLevel.MatchString(txt) {
			x := rgxSimcLevel.FindStringSubmatch(txt)
			i, err := strconv.Atoi(x[1])
			if err != nil {
				log.Fatalf("Unable to parse level '%v' for character: %v\n", x[1], err)
			}
			c.Level = i
			continue
		}
		if rgxSimcSpec.MatchString(txt) {
			x := rgxSimcSpec.FindStringSubmatch(txt)
			c.Spec = x[1]
			continue
		}
		if rgxSimcRace.MatchString(txt) {
			x := rgxSimcRace.FindStringSubmatch(txt)
			c.Race = x[1]
			continue
		}
		if rgxSimcRegion.MatchString(txt) {
			x := rgxSimcRegion.FindStringSubmatch(txt)
			c.Region = x[1]
			continue
		}
		if rgxSimcServer.MatchString(txt) {
			x := rgxSimcServer.FindStringSubmatch(txt)
			c.Server = x[1]
			continue
		}
		if rgxSimcRole.MatchString(txt) {
			continue
		}
		if rgxSimcProfessions.MatchString(txt) {
			x := rgxSimcProfessions.FindStringSubmatch(txt)
			c.Professions = x[1]
			continue
		}
		if rgxSimcTalents.MatchString(txt) {
			x := rgxSimcTalents.FindStringSubmatch(txt)
			c.Talents = make([]int, 0)
			for _, x := range x[1:] {
				y, err := strconv.Atoi(x)
				if err != nil {
					log.Fatalf("Should be an unreachable error!: %v\n", err)
				}
				c.Talents = append(c.Talents, y)
			}
			continue
		}
		if rgxSimcArtifact.MatchString(txt) {
			x := rgxSimcArtifact.FindStringSubmatch(txt)
			arr := strings.Split(x[1], ":")
			c.Artifact = make([]struct {
				a int
				b int
			}, 0)
			for i := 0; i+1 < len(arr); i += 2 {
				a, _ := strconv.Atoi(arr[i])
				b, _ := strconv.Atoi(arr[i+1])
				c.Artifact = append(c.Artifact, struct {
					a int
					b int
				}{a, b})
			}
			continue
		}
		if rgxSimcCrucible.MatchString(txt) {
			x := rgxSimcCrucible.FindStringSubmatch(txt)
			y := strings.Split(x[1], "/")
			c.Crucible = make([]struct {
				a int
				b int
			}, 0)
			for _, v := range y {
				z := strings.Split(v, ":")
				if len(z) != 2 {
					continue
					log.Fatalf("Unable to parse crucible data\n")
				}
				a, _ := strconv.Atoi(z[0])
				b, _ := strconv.Atoi(z[1])
				c.Crucible = append(c.Crucible, struct {
					a int
					b int
				}{a, b})
			}
			continue
		}
		fmt.Printf("Unable to parse line from simc: `%v`\n", txt)
	}

	wg := sync.WaitGroup{}
	wg.Add(len(c.Items))
	for i, v := range c.Items {
		go func(i int, itm ItemInfo) {
			defer wg.Done()
			strs := make([]string, len(itm.BonusIDs))
			for i, v := range itm.BonusIDs {
				strs[i] = strconv.Itoa(v)
			}
			url := fmt.Sprintf(`https://us.api.battle.net/wow/item/%v?locale=%v&bl=%v&apikey=%v`, itm.ID, "en_US", strings.Join(strs, ","), wowAPIKey)
			resp, err := http.Get(url)
			if err != nil {
				log.Fatalf("Unable to get url '%v': %v\n", url, err)
			}
			data, _ := ioutil.ReadAll(resp.Body)
			err = json.Unmarshal(data, &itm)
			if err != nil {
				//log.Fatalf("Unable to unmarshal response: %v\nResponse: %s\n", err, data)
			}
			if itm.Name == "Void Stalker's Contract" {
				fmt.Println(itm.Stats)
			}
			c.Items[i] = itm
		}(i, v)
	}
	wg.Wait()
	return
}
