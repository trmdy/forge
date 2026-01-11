package names

import (
	"fmt"
	"math/rand"
	"time"
)

var baseAdjectives = []string{
	"agile", "amped", "atomic", "beefy", "bold", "brisk", "bright", "brutal",
	"calm", "clean", "clever", "cool", "crisp", "daring", "dashing", "deft",
	"dynamic", "eager", "electric", "epic", "exact", "fearless", "fierce", "fiery",
	"flashy", "focused", "frank", "friendly", "frisky", "fun", "furious", "gallant",
	"game", "gentle", "gifted", "glorious", "golden", "gritty", "grounded", "happy",
	"hardy", "hearty", "heroic", "honest", "humble", "hungry", "icy", "jaunty",
	"keen", "kind", "laser", "lively", "lucid", "lucky", "lunar", "mellow",
	"merry", "mighty", "modest", "nimble", "noble", "open", "patient", "peppy",
	"perky", "playful", "polished", "punchy", "quick", "quiet", "radiant", "rapid",
	"ready", "regal", "relentless", "resolute", "restless", "robust", "rosy", "rugged",
	"savage", "savvy", "serene", "sharp", "shrewd", "slick", "solid", "spry",
	"steady", "stellar", "stoic", "sturdy", "sunny", "swift", "tactful", "tidy",
	"tough", "tranquil", "trusty", "upbeat", "valiant", "vivid", "warm", "wild",
	"witty", "zany", "zesty", "zippy", "zealous", "brave", "breezy", "buoyant",
	"canny", "chipper", "classic", "cosmic", "crafty", "fleet", "fresh", "jolly",
}

var givenNames = []string{
	"homer", "marge", "bart", "lisa", "maggie", "abe", "ned", "maude",
	"rod", "todd", "milhouse", "martin", "nelson", "ralph", "clancy", "seymour",
	"edna", "waylon", "monty", "apu", "lenny", "carl", "moe", "barney",
	"willie", "troy", "kent", "bob", "krusty", "otto", "patty", "selma",
	"agnes", "frink", "jacqueline", "luann", "jimbo", "dolph", "kearney", "gil",
	"stan", "kyle", "eric", "kenny", "butters", "wendy", "randy", "sharon",
	"shelley", "ike", "tweek", "craig", "clyde", "jimmy", "timmy", "pip",
	"henrietta", "bebe", "red", "token", "garrison", "mackey", "chef", "gerald",
	"sheila", "liane",
	"peter", "lois", "meg", "chris", "stewie", "brian", "glenn", "cleveland",
	"joe", "bonnie", "mort", "herbert", "consuela", "adam", "carter", "tom",
	"jerome", "ida", "rupert", "jillian", "tricia", "seamus", "patrick", "bertram",
}

var familyNames = []string{
	"simpson", "flanders", "burns", "smithers", "wiggum", "vanhouten", "skinner", "krabappel",
	"hibbert", "lovejoy", "brockman", "szyslak", "gumble", "frink", "quimby", "prince",
	"tatum", "terwilliger", "spuckler", "bouvier", "cho", "muntz", "nahasapeemapetilon", "wolfcastle",
	"krustofsky", "mcclure", "chalmers", "jones", "kearney", "landers",
	"marsh", "broflovski", "mccormick", "cartman", "stotch", "garrison", "mackey", "valmer",
	"tucker", "black", "testaburger", "tweek", "donovan", "anderson", "mcrae",
	"griffin", "quagmire", "brown", "swanson", "pewterschmidt", "goldman", "west",
	"simmons", "takanawa", "quahog", "pawtucket", "spooner", "longbottom", "mccoy",
}

var extraSingleNames = []string{
	"itchy", "scratchy", "snowball", "santaslittlehelper", "comicbook", "towelie",
	"manbearpig", "giantchicken", "greasedupdeafguy",
}

var singleNames = buildSingleNames()

// RandomLoopNameTwoPart returns a kebab-case adjective/name combo.
func RandomLoopNameTwoPart(rng *rand.Rand) string {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	adjective := randomAdjective(rng)
	name := singleNames[rng.Intn(len(singleNames))]
	return fmt.Sprintf("%s-%s", adjective, name)
}

// RandomLoopNameThreePart returns a kebab-case adjective/given/family combo.
func RandomLoopNameThreePart(rng *rand.Rand) string {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	adjective := randomAdjective(rng)
	left := givenNames[rng.Intn(len(givenNames))]
	right := familyNames[rng.Intn(len(familyNames))]
	return fmt.Sprintf("%s-%s-%s", adjective, left, right)
}

// LoopNameCountTwoPart returns the number of possible adjective/name combinations.
func LoopNameCountTwoPart() int {
	return adjectiveCount() * len(singleNames)
}

// LoopNameCountThreePart returns the number of possible adjective/given/family combinations.
func LoopNameCountThreePart() int {
	return adjectiveCount() * len(givenNames) * len(familyNames)
}

// LoopNameCount returns the total number of possible combinations.
func LoopNameCount() int {
	return LoopNameCountTwoPart() + LoopNameCountThreePart()
}

func adjectiveCount() int {
	return len(baseAdjectives)
}

func randomAdjective(rng *rand.Rand) string {
	return baseAdjectives[rng.Intn(len(baseAdjectives))]
}

func buildSingleNames() []string {
	tokens := make([]string, 0, len(givenNames)+len(familyNames)+len(extraSingleNames))
	tokens = append(tokens, givenNames...)
	tokens = append(tokens, familyNames...)
	tokens = append(tokens, extraSingleNames...)
	return uniqueTokens(tokens)
}

func uniqueTokens(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	unique := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		unique = append(unique, token)
	}
	return unique
}
