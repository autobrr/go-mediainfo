package mediainfo

import internalmediainfo "github.com/autobrr/go-mediainfo/internal/mediainfo"

func RenderText(reports []Report) string {
	return internalmediainfo.RenderText(reports)
}

func RenderJSON(reports []Report) string {
	return internalmediainfo.RenderJSON(reports)
}

func RenderXML(reports []Report) string {
	return internalmediainfo.RenderXML(reports)
}

func RenderHTML(reports []Report) string {
	return internalmediainfo.RenderHTML(reports)
}

func RenderCSV(reports []Report) string {
	return internalmediainfo.RenderCSV(reports)
}

func RenderEBUCore(reports []Report) string {
	return internalmediainfo.RenderEBUCore(reports)
}

func RenderPBCore(reports []Report) string {
	return internalmediainfo.RenderPBCore(reports)
}

func RenderGraphSVG(reports []Report) string {
	return internalmediainfo.RenderGraphSVG(reports)
}

func RenderGraphDOT(reports []Report) string {
	return internalmediainfo.RenderGraphDOT(reports)
}
