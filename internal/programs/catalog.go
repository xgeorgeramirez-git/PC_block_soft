package programs

import "strings"

// Категории программ (как programs.category и ipc.ProgramCat*).
const (
	CatGames   = "games"
	CatStudy   = "study"
	CatOther   = "other"
	CatBlocked = "blocked"
	CatSystem  = "system"
)

// Catalog — встроенный каталог известных игр и браузеров по имени файла exe в нижнем регистре
// (R12: «известные игры и браузеры — по встроенному каталогу»). Значение — название для людей.
// Действует только для путей в доверенных каталогах (RM3-1 п.4): переименованный exe в
// «Загрузках» каталогом не узнаётся.
type Catalog struct {
	Games    map[string]string
	Browsers map[string]string
	// GameFamilies — игры Microsoft Store по имени пакета без версии и издателя
	// («microsoft.minecraftuwp»): действует только в %ProgramFiles%\WindowsApps (Д-3).
	GameFamilies map[string]string
}

// DefaultCatalog — каталог программы.
var DefaultCatalog = Catalog{Games: catalogGames, Browsers: catalogBrowsers, GameFamilies: catalogGameFamilies}

// Lookup ищет программу по имени файла (или по пути: берётся имя файла; регистр не важен).
// Игра — CatGames; браузер — CatOther (в M3 сайты не различаются, RM3-2). Нет в каталоге — "", false.
func (c Catalog) Lookup(name string) (category string, ok bool) {
	base := baseName(name)
	if base == "" {
		return "", false
	}
	if _, ok := c.Games[base]; ok {
		return CatGames, true
	}
	if _, ok := c.Browsers[base]; ok {
		return CatOther, true
	}
	return "", false
}

// LookupFamily ищет игру Microsoft Store по имени пакета (без версии и издателя, регистр не
// важен). Нет в каталоге — "", false.
func (c Catalog) LookupFamily(pkgName string) (category string, ok bool) {
	if _, ok := c.GameFamilies[strings.ToLower(pkgName)]; ok && pkgName != "" {
		return CatGames, true
	}
	return "", false
}

// IsBrowser сообщает, что файл — известный браузер (для сайтов в M4).
func (c Catalog) IsBrowser(name string) bool {
	_, ok := c.Browsers[baseName(name)]
	return ok
}

// baseName — имя файла из имени или пути в нижнем регистре.
func baseName(name string) string {
	p := strings.ToLower(strings.ReplaceAll(name, "/", `\`))
	return p[strings.LastIndex(p, `\`)+1:]
}

// catalogBrowsers — известные браузеры.
var catalogBrowsers = map[string]string{
	"brave.exe":    "Brave",
	"chrome.exe":   "Google Chrome",
	"msedge.exe":   "Microsoft Edge",
	"firefox.exe":  "Mozilla Firefox",
	"opera.exe":    "Opera",
	"vivaldi.exe":  "Vivaldi",
	"iexplore.exe": "Internet Explorer",
	"browser.exe":  "Яндекс Браузер",
}

// catalogGameFamilies — игры Microsoft Store по имени пакета (Д-3); имена exe внутри пакетов
// дополнительно узнаются по catalogGames (minecraft.windows.exe).
var catalogGameFamilies = map[string]string{
	"microsoft.minecraftuwp":                 "Minecraft for Windows",
	"microsoft.minecraftjavaedition":         "Minecraft Launcher",
	"microsoft.minecraftwindowsbeta":         "Minecraft Preview",
	"robloxcorporation.roblox":               "Roblox",
	"roblox":                                 "Roblox",
	"microsoft.microsoftsolitairecollection": "Microsoft Solitaire Collection",
}

// catalogGames — известные игры и игровые лаунчеры. javaw.exe сюда не входит: Java запускает не
// только Minecraft, но и учебные программы.
var catalogGames = map[string]string{
	"minecraftlauncher.exe":             "Minecraft Launcher",
	"minecraft.exe":                     "Minecraft (старый лаунчер)",
	"minecraft.windows.exe":             "Minecraft for Windows",
	"tlauncher.exe":                     "TLauncher (Minecraft)",
	"robloxplayerbeta.exe":              "Roblox",
	"robloxplayerlauncher.exe":          "Roblox (запуск)",
	"steam.exe":                         "Steam",
	"epicgameslauncher.exe":             "Epic Games Launcher",
	"fortniteclient-win64-shipping.exe": "Fortnite",
	"fortnitelauncher.exe":              "Fortnite (запуск)",
	"valorant.exe":                      "Valorant",
	"valorant-win64-shipping.exe":       "Valorant",
	"riotclientservices.exe":            "Riot Client",
	"leagueclient.exe":                  "League of Legends (клиент)",
	"league of legends.exe":             "League of Legends",
	"genshinimpact.exe":                 "Genshin Impact",
	"starrail.exe":                      "Honkai: Star Rail",
	"zenlesszonezero.exe":               "Zenless Zone Zero",
	"gta5.exe":                          "Grand Theft Auto V",
	"playgtav.exe":                      "Grand Theft Auto V (запуск)",
	"csgo.exe":                          "Counter-Strike: Global Offensive",
	"cs2.exe":                           "Counter-Strike 2",
	"dota2.exe":                         "Dota 2",
	"hl2.exe":                           "Half-Life 2 и игры Source",
	"tf_win64.exe":                      "Team Fortress 2",
	"left4dead2.exe":                    "Left 4 Dead 2",
	"terraria.exe":                      "Terraria",
	"among us.exe":                      "Among Us",
	"osu!.exe":                          "osu!",
	"hearthstone.exe":                   "Hearthstone",
	"wow.exe":                           "World of Warcraft",
	"overwatch.exe":                     "Overwatch",
	"battle.net.exe":                    "Battle.net",
	"brawlhalla.exe":                    "Brawlhalla",
	"rocketleague.exe":                  "Rocket League",
	"fallguys_client_game.exe":          "Fall Guys",
	"origin.exe":                        "Origin",
	"eadesktop.exe":                     "EA app",
	"ubisoftconnect.exe":                "Ubisoft Connect",
	"upc.exe":                           "Ubisoft Connect (старое имя)",
	"galaxyclient.exe":                  "GOG Galaxy",
	"tslgame.exe":                       "PUBG: Battlegrounds",
	"r5apex.exe":                        "Apex Legends",
	"r5apex_dx12.exe":                   "Apex Legends",
	"rainbowsix.exe":                    "Rainbow Six Siege",
	"destiny2.exe":                      "Destiny 2",
	"warframe.x64.exe":                  "Warframe",
	"worldoftanks.exe":                  "World of Tanks",
	"wgc.exe":                           "Wargaming Game Center",
	"lgc.exe":                           "Lesta Game Center",
	"aces.exe":                          "War Thunder",
	"ts4_x64.exe":                       "The Sims 4",
	"fc24.exe":                          "EA Sports FC 24",
	"fc25.exe":                          "EA Sports FC 25",
	"geometrydash.exe":                  "Geometry Dash",
	"subnautica.exe":                    "Subnautica",
	"rustclient.exe":                    "Rust",
	"eldenring.exe":                     "Elden Ring",
	"witcher3.exe":                      "The Witcher 3",
	"cyberpunk2077.exe":                 "Cyberpunk 2077",
	"stardew valley.exe":                "Stardew Valley",
	"hollow_knight.exe":                 "Hollow Knight",
	"cuphead.exe":                       "Cuphead",
	"eurotrucks2.exe":                   "Euro Truck Simulator 2",
	"palworld-win64-shipping.exe":       "Palworld",
	"helldivers2.exe":                   "Helldivers 2",
	"bg3.exe":                           "Baldur's Gate 3",
	"bg3_dx11.exe":                      "Baldur's Gate 3",
	"vrchat.exe":                        "VRChat",
	"lethal company.exe":                "Lethal Company",
	"phasmophobia.exe":                  "Phasmophobia",
}
