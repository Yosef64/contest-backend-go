package docx

import (
	"regexp"
	"strings"
	"unicode"
)

// Word equations (Office Math, OMML) as LaTeX for the app's formula renderer.

var symbols = map[rune]string{
	'α': `\alpha`, 'β': `\beta`, 'γ': `\gamma`, 'δ': `\delta`, 'ε': `\varepsilon`,
	'ϵ': `\epsilon`, 'ζ': `\zeta`, 'η': `\eta`, 'θ': `\theta`, 'ϑ': `\vartheta`,
	'ι': `\iota`, 'κ': `\kappa`, 'λ': `\lambda`, 'μ': `\mu`, 'ν': `\nu`, 'ξ': `\xi`,
	'π': `\pi`, 'ρ': `\rho`, 'σ': `\sigma`, 'τ': `\tau`, 'υ': `\upsilon`, 'φ': `\varphi`,
	'ϕ': `\phi`, 'χ': `\chi`, 'ψ': `\psi`, 'ω': `\omega`, 'Γ': `\Gamma`, 'Δ': `\Delta`,
	'Θ': `\Theta`, 'Λ': `\Lambda`, 'Ξ': `\Xi`, 'Π': `\Pi`, 'Σ': `\Sigma`, 'Φ': `\Phi`,
	'Ψ': `\Psi`, 'Ω': `\Omega`, '∞': `\infty`, '±': `\pm`, '∓': `\mp`, '×': `\times`,
	'÷': `\div`, '·': `\cdot`, '⋅': `\cdot`, '∙': `\cdot`, '≤': `\le`, '≥': `\ge`,
	'≠': `\ne`, '≈': `\approx`, '≡': `\equiv`, '∝': `\propto`, '→': `\to`,
	'←': `\leftarrow`, '⇒': `\Rightarrow`, '⇔': `\Leftrightarrow`, '↔': `\leftrightarrow`,
	'∑': `\sum`, '∏': `\prod`, '∫': `\int`, '∮': `\oint`, '∂': `\partial`, '∇': `\nabla`,
	'√': `\surd`, '°': `^{\circ}`, '∠': `\angle`, '⊥': `\perp`, '∥': `\parallel`,
	'∈': `\in`, '∉': `\notin`, '⊂': `\subset`, '∪': `\cup`, '∩': `\cap`, '∅': `\emptyset`,
	'…': `\ldots`, '⋯': `\cdots`, '′': `'`, '″': `''`, '−': `-`, '–': `-`, '∗': `*`,
	'ℏ': `\hbar`, 'ℓ': `\ell`, '∴': `\therefore`, '∵': `\because`, '⟹': `\Longrightarrow`,
}

var funcs = map[string]bool{
	"sin": true, "cos": true, "tan": true, "cot": true, "sec": true, "csc": true, "log": true,
	"ln": true, "exp": true, "lim": true, "max": true, "min": true, "sinh": true, "cosh": true,
	"tanh": true, "arcsin": true, "arccos": true, "arctan": true, "det": true,
}

var accents = map[string]string{
	"⃗": `\vec`, "̂": `\hat`, "̄": `\bar`, "̅": `\overline`,
	"̇": `\dot`, "̈": `\ddot`, "̃": `\tilde`, "⃖": `\overleftarrow`,
	"→": `\vec`, "^": `\hat`, "~": `\tilde`,
}

var nary = map[string]string{
	"∑": `\sum`, "∏": `\prod`, "∫": `\int`, "∬": `\iint`, "∭": `\iiint`, "∮": `\oint`,
	"⋃": `\bigcup`, "⋂": `\bigcap`,
}

var delims = map[string]string{
	"(": "(", ")": ")", "[": "[", "]": "]", "{": `\{`, "}": `\}`, "|": "|",
	"‖": `\|`, "⟨": `\langle`, "⟩": `\rangle`, "⌊": `\lfloor`, "⌋": `\rfloor`,
	"⌈": `\lceil`, "⌉": `\rceil`, "": ".",
}

func delim(s string) string {
	if d, ok := delims[s]; ok {
		return d
	}
	return s
}

func isCommandWord(s string) bool {
	if !strings.HasPrefix(s, `\`) || len(s) < 2 {
		return false
	}
	for _, r := range s[1:] {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func texText(s string) string {
	var b strings.Builder
	for _, ch := range s {
		if sym, ok := symbols[ch]; ok {
			b.WriteString(sym)
			if isCommandWord(sym) {
				b.WriteByte(' ')
			}
			continue
		}
		switch ch {
		case '#', '$', '%', '&', '_', '{', '}':
			b.WriteByte('\\')
			b.WriteRune(ch)
		case '\\':
			b.WriteString(`\backslash `)
		case '^':
			b.WriteString(`\hat{}`)
		case '~':
			b.WriteString(`\sim `)
		case ' ', ' ':
			b.WriteString(`\ `)
		default:
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// chr reads a character property (m:begChr, m:chr...): def when the
// property is absent, its value (possibly empty) when present.
func chr(pr *node, name, def string) string {
	c := pr.child(name)
	if c == nil {
		return def
	}
	return c.attr("m:val")
}

var plainWord = regexp.MustCompile(`^[A-Za-z]{2,}$`)

// omml is the LaTeX for an equation element or any part of it.
func omml(el *node) string {
	sub := func(name string) string {
		if c := el.child("m:" + name); c != nil {
			return omml(c)
		}
		return ""
	}
	all := func() string {
		var b strings.Builder
		for _, k := range el.kids {
			b.WriteString(omml(k))
		}
		return b.String()
	}
	if el.space == nsM {
		switch el.local {
		case "oMath", "e", "num", "den", "sub", "sup", "deg", "fName", "lim":
			return all()
		case "r":
			t := el.texts("m:t")
			if t == "" {
				t = el.texts("w:t")
			}
			plain := el.val("m:rPr/m:sty") == "p" || el.child("m:rPr/m:nor") != nil
			word := strings.TrimSpace(t)
			if funcs[word] {
				return `\` + word + " "
			}
			tex := texText(t)
			if plain && plainWord.MatchString(word) {
				return `\mathrm{` + tex + `}`
			}
			return tex
		case "f":
			num, den := sub("num"), sub("den")
			switch el.val("m:fPr/m:type") {
			case "lin":
				return num + "/" + den
			case "noBar":
				return `\genfrac{}{}{0pt}{}{` + num + `}{` + den + `}`
			}
			return `\frac{` + num + `}{` + den + `}`
		case "sSup":
			return "{" + sub("e") + "}^{" + sub("sup") + "}"
		case "sSub":
			return "{" + sub("e") + "}_{" + sub("sub") + "}"
		case "sSubSup":
			return "{" + sub("e") + "}_{" + sub("sub") + "}^{" + sub("sup") + "}"
		case "sPre":
			return "{}_{" + sub("sub") + "}^{" + sub("sup") + "}{" + sub("e") + "}"
		case "rad":
			deg := sub("deg")
			hide := el.val("m:radPr/m:degHide")
			if deg != "" && hide != "1" && hide != "on" && hide != "true" {
				return `\sqrt[` + deg + `]{` + sub("e") + `}`
			}
			return `\sqrt{` + sub("e") + `}`
		case "d":
			pr := el.child("m:dPr")
			beg, end, sep := chr(pr, "m:begChr", "("), chr(pr, "m:endChr", ")"), chr(pr, "m:sepChr", "|")
			if sep == "" {
				sep = "|"
			}
			var items []string
			for _, e := range el.children("m:e") {
				items = append(items, omml(e))
			}
			return `\left` + delim(beg) + strings.Join(items, " "+delim(sep)+" ") + `\right` + delim(end)
		case "nary":
			op := chr(el.child("m:naryPr"), "m:chr", "∫")
			if op == "" {
				op = "∫"
			}
			tex, ok := nary[op]
			if !ok {
				tex = texText(op)
			}
			if lo := sub("sub"); lo != "" {
				tex += "_{" + lo + "}"
			}
			if hi := sub("sup"); hi != "" {
				tex += "^{" + hi + "}"
			}
			return tex + " " + sub("e")
		case "acc":
			c := chr(el.child("m:accPr"), "m:chr", "̂")
			cmd, ok := accents[c]
			if !ok {
				cmd = `\hat`
			}
			return cmd + "{" + sub("e") + "}"
		case "bar":
			if el.val("m:barPr/m:pos") == "bot" {
				return `\underline{` + sub("e") + "}"
			}
			return `\overline{` + sub("e") + "}"
		case "func":
			name := strings.TrimSpace(sub("fName"))
			if funcs[strings.ReplaceAll(name, " ", "")] {
				name = `\` + strings.ReplaceAll(name, " ", "")
			}
			return name + " " + sub("e")
		case "limLow":
			return sub("e") + "_{" + sub("lim") + "}"
		case "limUpp":
			return sub("e") + "^{" + sub("lim") + "}"
		case "groupChr":
			if el.val("m:groupChrPr/m:pos") == "top" {
				return `\overbrace{` + sub("e") + "}"
			}
			return `\underbrace{` + sub("e") + "}"
		case "eqArr":
			var rows []string
			for _, e := range el.children("m:e") {
				rows = append(rows, strings.ReplaceAll(omml(e), "&", ""))
			}
			return `\begin{aligned}` + strings.Join(rows, ` \\ `) + `\end{aligned}`
		case "m":
			var rows []string
			for _, mr := range el.children("m:mr") {
				var cells []string
				for _, e := range mr.children("m:e") {
					cells = append(cells, omml(e))
				}
				rows = append(rows, strings.Join(cells, " & "))
			}
			return `\begin{matrix}` + strings.Join(rows, ` \\ `) + `\end{matrix}`
		case "box", "borderBox", "phant":
			return sub("e")
		case "ctrlPr":
			return ""
		}
		if strings.HasSuffix(el.local, "Pr") {
			return ""
		}
		return all()
	}
	if el.is("w:r") { // text runs inside equations
		return texText(el.texts("w:t"))
	}
	if el.space == nsW {
		return ""
	}
	return all()
}

var (
	repeatedSpaces = regexp.MustCompile(`(\\ ){2,}`)
	endSpaces      = regexp.MustCompile(`^(?:\\ |\s)+|(?:\\ |\s)+$`)
)

// tidyTex drops repeated spacing and spacing at the ends (a leading \left
// must stay).
func tidyTex(tex string) string {
	tex = repeatedSpaces.ReplaceAllString(tex, `\ `)
	return endSpaces.ReplaceAllString(tex, "")
}
