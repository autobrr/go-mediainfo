package mediainfo

import (
	"fmt"
	"strings"
)

var languageNames = map[string]string{
	"aa":  "Afar",
	"ab":  "Abkhazian",
	"ae":  "Avestan",
	"af":  "Afrikaans",
	"ak":  "Akan",
	"am":  "Amharic",
	"an":  "Aragonese",
	"ar":  "Arabic",
	"as":  "Assamese",
	"av":  "Avaric",
	"ay":  "Aymara",
	"az":  "Azerbaijani",
	"ba":  "Bashkir",
	"be":  "Belarusian",
	"bg":  "Bulgarian",
	"bh":  "Bihari languages",
	"bi":  "Bislama",
	"bm":  "Bambara",
	"bn":  "Bengali",
	"bo":  "Tibetan",
	"br":  "Breton",
	"bs":  "Bosnian",
	"ca":  "Catalan",
	"ce":  "Chechen",
	"ch":  "Chamorro",
	"cmn": "Mandarin",
	"co":  "Corsican",
	"cr":  "Cree",
	"cs":  "Czech",
	"cu":  "Church Slavic",
	"cv":  "Chuvash",
	"cy":  "Welsh",
	"da":  "Danish",
	"de":  "German",
	"dv":  "Divehi",
	"dz":  "Dzongkha",
	"ee":  "Ewe",
	"el":  "Greek",
	"en":  "English",
	"eo":  "Esperanto",
	"es":  "Spanish",
	"et":  "Estonian",
	"eu":  "Basque",
	"fa":  "Persian",
	"ff":  "Fulah",
	"fi":  "Finnish",
	"fil": "Filipino",
	"fj":  "Fijian",
	"fo":  "Faroese",
	"fr":  "French",
	"fy":  "Western Frisian",
	"ga":  "Irish",
	"gd":  "Gaelic",
	"gl":  "Galician",
	"gn":  "Guarani",
	"gu":  "Gujarati",
	"gv":  "Manx",
	"ha":  "Hausa",
	"he":  "Hebrew",
	"hi":  "Hindi",
	"ho":  "Hiri Motu",
	"hr":  "Croatian",
	"ht":  "Haitian",
	"hu":  "Hungarian",
	"hy":  "Armenian",
	"hz":  "Herero",
	"ia":  "Interlingua",
	"id":  "Indonesian",
	"ie":  "Interlingue",
	"ig":  "Igbo",
	"ii":  "Sichuan Yi",
	"ik":  "Inupiaq",
	"io":  "Ido",
	"is":  "Icelandic",
	"it":  "Italian",
	"iu":  "Inuktitut",
	"ja":  "Japanese",
	"jv":  "Javanese",
	"ka":  "Georgian",
	"kg":  "Kongo",
	"ki":  "Kikuyu",
	"kj":  "Kuanyama",
	"kk":  "Kazakh",
	"kl":  "Kalaallisut",
	"km":  "Central Khmer",
	"kn":  "Kannada",
	"ko":  "Korean",
	"kr":  "Kanuri",
	"ks":  "Kashmiri",
	"ku":  "Kurdish",
	"kv":  "Komi",
	"kw":  "Cornish",
	"ky":  "Kirghiz",
	"la":  "Latin",
	"lb":  "Luxembourgish",
	"lg":  "Ganda",
	"li":  "Limburgan",
	"ln":  "Lingala",
	"lo":  "Lao",
	"lt":  "Lithuanian",
	"lu":  "Luba-Katanga",
	"lv":  "Latvian",
	"mg":  "Malagasy",
	"mh":  "Marshallese",
	"mi":  "Maori",
	"mk":  "Macedonian",
	"ml":  "Malayalam",
	"mn":  "Mongolian",
	"mr":  "Marathi",
	"ms":  "Malay",
	"mt":  "Maltese",
	"my":  "Burmese",
	"na":  "Nauru",
	"nb":  "Norwegian Bokmal",
	"nd":  "North Ndebele",
	"ne":  "Nepali",
	"ng":  "Ndonga",
	"nl":  "Dutch",
	"nn":  "Norwegian Nynorsk",
	"no":  "Norwegian",
	"nr":  "South Ndebele",
	"nv":  "Navajo",
	"ny":  "Chichewa",
	"oc":  "Occitan",
	"oj":  "Ojibwa",
	"om":  "Oromo",
	"or":  "Oriya",
	"os":  "Ossetian",
	"pa":  "Panjabi",
	"pi":  "Pali",
	"pl":  "Polish",
	"ps":  "Pashto",
	"pt":  "Portuguese",
	"qu":  "Quechua",
	"rm":  "Romansh",
	"rn":  "Rundi",
	"ro":  "Romanian",
	"ru":  "Russian",
	"rw":  "Kinyarwanda",
	"sa":  "Sanskrit",
	"sc":  "Sardinian",
	"sd":  "Sindhi",
	"se":  "Northern Sami",
	"sg":  "Sango",
	"si":  "Sinhala",
	"sk":  "Slovak",
	"sl":  "Slovenian",
	"sm":  "Samoan",
	"sn":  "Shona",
	"so":  "Somali",
	"sq":  "Albanian",
	"sr":  "Serbian",
	"ss":  "Swati",
	"st":  "Southern Sotho",
	"su":  "Sundanese",
	"sv":  "Swedish",
	"sw":  "Swahili",
	"ta":  "Tamil",
	"te":  "Telugu",
	"tg":  "Tajik",
	"th":  "Thai",
	"ti":  "Tigrinya",
	"tk":  "Turkmen",
	"tl":  "Tagalog",
	"tn":  "Tswana",
	"to":  "Tonga",
	"tr":  "Turkish",
	"ts":  "Tsonga",
	"tt":  "Tatar",
	"tw":  "Twi",
	"ty":  "Tahitian",
	"ug":  "Uighur",
	"uk":  "Ukrainian",
	"ur":  "Urdu",
	"uz":  "Uzbek",
	"ve":  "Venda",
	"vi":  "Vietnamese",
	"vo":  "Volapuk",
	"wa":  "Walloon",
	"wo":  "Wolof",
	"xh":  "Xhosa",
	"yi":  "Yiddish",
	"yo":  "Yoruba",
	"yue": "Cantonese",
	"za":  "Zhuang",
	"zh":  "Chinese",
	"zu":  "Zulu",
}

var languageDisplayOverrides = map[string]string{
	"es-419":  "Spanish (Latin America)",
	"zh-Hans": "Chinese (Simplified)",
	"zh-Hant": "Chinese (Traditional)",
	"zh-TW":   "Chinese (Taiwan)",
}

var languageMap3To2 = map[string]string{
	"aar": "aa",
	"abk": "ab",
	"ave": "ae",
	"afr": "af",
	"aka": "ak",
	"amh": "am",
	"arg": "an",
	"ara": "ar",
	"asm": "as",
	"ava": "av",
	"aym": "ay",
	"aze": "az",
	"bak": "ba",
	"bel": "be",
	"bul": "bg",
	"bih": "bh",
	"bis": "bi",
	"bam": "bm",
	"ben": "bn",
	"bod": "bo",
	"tib": "bo",
	"bre": "br",
	"bos": "bs",
	"cat": "ca",
	"che": "ce",
	"cha": "ch",
	"cos": "co",
	"cre": "cr",
	"ces": "cs",
	"cze": "cs",
	"chu": "cu",
	"chv": "cv",
	"cym": "cy",
	"wel": "cy",
	"dan": "da",
	"deu": "de",
	"ger": "de",
	"div": "dv",
	"dzo": "dz",
	"ewe": "ee",
	"ell": "el",
	"gre": "el",
	"eng": "en",
	"epo": "eo",
	"spa": "es",
	"est": "et",
	"eus": "eu",
	"baq": "eu",
	"fas": "fa",
	"per": "fa",
	"ful": "ff",
	"fin": "fi",
	"fij": "fj",
	"fao": "fo",
	"fra": "fr",
	"fre": "fr",
	"fry": "fy",
	"gle": "ga",
	"gla": "gd",
	"glg": "gl",
	"grn": "gn",
	"guj": "gu",
	"glv": "gv",
	"hau": "ha",
	"heb": "he",
	"hin": "hi",
	"hmo": "ho",
	"hrv": "hr",
	"hat": "ht",
	"hun": "hu",
	"hye": "hy",
	"arm": "hy",
	"her": "hz",
	"ina": "ia",
	"ind": "id",
	"ile": "ie",
	"ibo": "ig",
	"iii": "ii",
	"ipk": "ik",
	"ido": "io",
	"isl": "is",
	"ice": "is",
	"ita": "it",
	"iku": "iu",
	"in":  "id",
	"iw":  "he",
	"ji":  "yi",
	"jpn": "ja",
	"jav": "jv",
	"kat": "ka",
	"geo": "ka",
	"kon": "kg",
	"kik": "ki",
	"kua": "kj",
	"kaz": "kk",
	"kal": "kl",
	"khm": "km",
	"kan": "kn",
	"kor": "ko",
	"kau": "kr",
	"kas": "ks",
	"kur": "ku",
	"kom": "kv",
	"cor": "kw",
	"kir": "ky",
	"lat": "la",
	"ltz": "lb",
	"lug": "lg",
	"lim": "li",
	"lin": "ln",
	"lao": "lo",
	"lit": "lt",
	"lub": "lu",
	"lav": "lv",
	"mlg": "mg",
	"mah": "mh",
	"mri": "mi",
	"mao": "mi",
	"mkd": "mk",
	"mac": "mk",
	"mal": "ml",
	"mon": "mn",
	"mar": "mr",
	"msa": "ms",
	"may": "ms",
	"mlt": "mt",
	"mya": "my",
	"bur": "my",
	"nau": "na",
	"nob": "nb",
	"nde": "nd",
	"nep": "ne",
	"ndo": "ng",
	"nld": "nl",
	"dut": "nl",
	"nno": "nn",
	"nor": "no",
	"nbl": "nr",
	"nav": "nv",
	"nya": "ny",
	"oci": "oc",
	"oji": "oj",
	"orm": "om",
	"ori": "or",
	"oss": "os",
	"pan": "pa",
	"pli": "pi",
	"pol": "pl",
	"pus": "ps",
	"por": "pt",
	"que": "qu",
	"roh": "rm",
	"run": "rn",
	"ron": "ro",
	"rum": "ro",
	"rus": "ru",
	"kin": "rw",
	"san": "sa",
	"srd": "sc",
	"snd": "sd",
	"sme": "se",
	"sag": "sg",
	"sin": "si",
	"slk": "sk",
	"slo": "sk",
	"slv": "sl",
	"smo": "sm",
	"sna": "sn",
	"som": "so",
	"sqi": "sq",
	"alb": "sq",
	"srp": "sr",
	"ssw": "ss",
	"sot": "st",
	"sun": "su",
	"swe": "sv",
	"swa": "sw",
	"tam": "ta",
	"tel": "te",
	"tgk": "tg",
	"tha": "th",
	"tir": "ti",
	"tuk": "tk",
	"tgl": "tl",
	"tsn": "tn",
	"ton": "to",
	"tur": "tr",
	"tso": "ts",
	"tat": "tt",
	"twi": "tw",
	"tah": "ty",
	"uig": "ug",
	"ukr": "uk",
	"urd": "ur",
	"uzb": "uz",
	"ven": "ve",
	"vie": "vi",
	"vol": "vo",
	"wln": "wa",
	"wol": "wo",
	"xho": "xh",
	"yid": "yi",
	"yor": "yo",
	"zha": "za",
	"zho": "zh",
	"chi": "zh",
	"zul": "zu",
}

var languageNames3 = map[string]string{
	"fil": "Filipino",
	"mis": "Uncoded languages",
	"mul": "Multiple languages",
	"tlh": "Klingon",
	"zxx": "Silent",
}

func normalizeLanguageCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	candidate := strings.ReplaceAll(code, "_", "-")
	parts := strings.Split(candidate, "-")
	if len(parts) == 0 || hasEmptyLanguageSubtag(parts) {
		return code
	}
	lang := strings.ToLower(parts[0])
	if lang == "und" {
		return ""
	}
	if !isAlpha(lang) || len(lang) < 2 || len(lang) > 8 {
		return code
	}
	if mapped, ok := languageMap3To2[lang]; ok {
		lang = mapped
	}
	out := []string{lang}
	i := 1
	for extlangCount := 0; i < len(parts) && extlangCount < 3; extlangCount++ {
		part := strings.ToLower(parts[i])
		if len(part) != 3 || !isAlpha(part) {
			break
		}
		out = append(out, part)
		i++
	}
	if i < len(parts) && len(parts[i]) == 4 && isAlpha(parts[i]) {
		part := parts[i]
		out = append(out, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
		i++
	}
	if i < len(parts) && isLanguageRegionSubtag(parts[i]) {
		out = append(out, strings.ToUpper(parts[i]))
		i++
	}
	seenVariants := map[string]struct{}{}
	for i < len(parts) && isLanguageVariantSubtag(parts[i]) {
		variant := strings.ToLower(parts[i])
		if _, ok := seenVariants[variant]; ok {
			return code
		}
		seenVariants[variant] = struct{}{}
		out = append(out, variant)
		i++
	}
	seenExtensions := map[string]struct{}{}
	for i < len(parts) && isLanguageExtensionSingleton(parts[i]) {
		singleton := strings.ToLower(parts[i])
		if _, ok := seenExtensions[singleton]; ok {
			return code
		}
		seenExtensions[singleton] = struct{}{}
		out = append(out, singleton)
		i++
		start := i
		for i < len(parts) && isLanguageExtensionSubtag(parts[i]) {
			out = append(out, strings.ToLower(parts[i]))
			i++
		}
		if i == start {
			return code
		}
	}
	if i < len(parts) && strings.EqualFold(parts[i], "x") {
		out = append(out, "x")
		i++
		start := i
		for i < len(parts) && isLanguagePrivateUseSubtag(parts[i]) {
			out = append(out, strings.ToLower(parts[i]))
			i++
		}
		if i == start {
			return code
		}
	}
	if i != len(parts) {
		return code
	}
	return strings.Join(out, "-")
}

func formatLanguage(code string) string {
	normalized := normalizeLanguageCode(code)
	if normalized == "" {
		return ""
	}

	if name := languageDisplayOverrides[normalized]; name != "" {
		return name
	}

	parts := strings.Split(normalized, "-")
	name := languageName(parts[0])
	if name == "" {
		return code
	}
	if len(parts) > 1 {
		return fmt.Sprintf("%s (%s)", name, strings.Join(parts[1:], "-"))
	}
	return name
}

func languageName(code string) string {
	if name := languageNames[code]; name != "" {
		return name
	}
	return languageNames3[code]
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

func isDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func hasEmptyLanguageSubtag(parts []string) bool {
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return true
		}
	}
	return false
}

func isAlphaNum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func isLanguageRegionSubtag(s string) bool {
	return len(s) == 2 && isAlpha(s) || len(s) == 3 && isDigit(s)
}

func isLanguageVariantSubtag(s string) bool {
	return len(s) >= 5 && len(s) <= 8 && isAlphaNum(s) || len(s) == 4 && isDigit(s[:1]) && isAlphaNum(s)
}

func isLanguageExtensionSingleton(s string) bool {
	return len(s) == 1 && isAlphaNum(s) && !strings.EqualFold(s, "x")
}

func isLanguageExtensionSubtag(s string) bool {
	return len(s) >= 2 && len(s) <= 8 && isAlphaNum(s)
}

func isLanguagePrivateUseSubtag(s string) bool {
	return len(s) >= 1 && len(s) <= 8 && isAlphaNum(s)
}
