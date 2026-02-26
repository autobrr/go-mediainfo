package mediainfo

import internalmediainfo "github.com/autobrr/go-mediainfo/internal/mediainfo"

// RenderText renders reports []Report as MediaInfo-style plain text output and returns the rendered string.
func RenderText(reports []Report) string {
	return internalmediainfo.RenderText(reports)
}

// RenderJSON renders reports []Report as MediaInfo JSON output and returns the rendered string.
func RenderJSON(reports []Report) string {
	return internalmediainfo.RenderJSON(reports)
}

// RenderXML renders reports []Report as XML output and returns the rendered string.
func RenderXML(reports []Report) string {
	return internalmediainfo.RenderXML(reports)
}

// RenderHTML renders reports []Report as HTML output and returns the rendered string.
func RenderHTML(reports []Report) string {
	return internalmediainfo.RenderHTML(reports)
}

// RenderCSV renders reports []Report as CSV output and returns the rendered string.
func RenderCSV(reports []Report) string {
	return internalmediainfo.RenderCSV(reports)
}

// RenderEBUCore renders reports []Report as EBUCore output and returns the rendered string.
func RenderEBUCore(reports []Report) string {
	return internalmediainfo.RenderEBUCore(reports)
}

// RenderPBCore renders reports []Report as PBCore output and returns the rendered string.
func RenderPBCore(reports []Report) string {
	return internalmediainfo.RenderPBCore(reports)
}

// RenderGraphSVG renders reports []Report as Graph SVG output and returns the rendered string.
func RenderGraphSVG(reports []Report) string {
	return internalmediainfo.RenderGraphSVG(reports)
}

// RenderGraphDOT renders reports []Report as Graph DOT output and returns the rendered string.
func RenderGraphDOT(reports []Report) string {
	return internalmediainfo.RenderGraphDOT(reports)
}
