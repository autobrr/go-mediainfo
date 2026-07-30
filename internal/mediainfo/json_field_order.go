package mediainfo

import "sort"

var jsonGeneralFieldOrder = map[string]int{
	"@type": 0, "ID": 1, "UniqueID": 1, "VideoCount": 2, "AudioCount": 3, "TextCount": 4, "ImageCount": 5, "MenuCount": 6,
	"FileExtension": 7, "CompleteName_Last": 7, "Format": 8, "Format_Settings": 9, "Format_Version": 10, "Format_Profile": 11,
	"CodecID": 12, "CodecID_Compatible": 13, "FileSize": 14, "Duration": 15, "OverallBitRate_Mode": 16, "OverallBitRate": 17,
	"FrameRate": 18, "FrameCount": 19, "StreamSize": 20, "HeaderSize": 21, "DataSize": 22, "FooterSize": 23, "IsStreamable": 24,
	"File_Created_Date": 25, "File_Created_Date_Local": 26, "File_Modified_Date": 27, "File_Modified_Date_Local": 28,
	"Encoded_Application": 29, "Encoded_Application_Name": 30, "Encoded_Application_Version": 31,
	"Encoded_Library": 32, "Encoded_Library_Name": 33, "Encoded_Library_Version": 34, "Encoded_Library_Settings": 35, "extra": 36,
}

var jsonVideoFieldOrder = map[string]int{
	"@type": 0, "@typeorder": 1, "StreamOrder": 2, "FirstPacketOrder": 3, "ID": 4, "MenuID": 5, "UniqueID": 6,
	"Format": 7, "Format_Version": 8, "Format_Profile": 9, "Format_Level": 10, "Format_Tier": 11,
	"Format_Settings_CABAC": 12, "Format_Settings_RefFrames": 13, "Format_Settings_BVOP": 14, "Format_Settings_QPel": 14, "Format_Settings_GMC": 14,
	"Format_Settings_Matrix": 15, "Format_Settings_Matrix_Data": 16, "Format_Settings_GOP": 17, "Format_Settings_PictureStructure": 17,
	"CodecID": 18, "Duration": 19, "BitRate_Mode": 20, "BitRate": 21, "BitRate_Nominal": 22, "BitRate_Maximum": 23,
	"Width": 24, "Height": 25, "Stored_Width": 26, "Stored_Height": 27, "Sampled_Width": 28, "Sampled_Height": 29,
	"PixelAspectRatio": 30, "DisplayAspectRatio": 31, "Rotation": 32, "FrameRate_Mode": 33, "FrameRate_Mode_Original": 34,
	"FrameRate": 35, "FrameRate_Num": 36, "FrameRate_Den": 37, "FrameCount": 38, "Standard": 39, "ColorSpace": 40,
	"ChromaSubsampling": 41, "ChromaSubsampling_Position": 42, "BitDepth": 43, "ScanType": 44, "Compression_Mode": 45,
	"Delay": 46, "Delay_Settings": 47, "Delay_DropFrame": 48, "Delay_Source": 49, "Delay_Original": 50,
	"Delay_Original_DropFrame": 51, "Delay_Original_Source": 52, "TimeCode_FirstFrame": 53, "TimeCode_Source": 54,
	"Gop_OpenClosed": 55, "Gop_OpenClosed_FirstFrame": 56, "StreamSize": 57, "Encoded_Library": 58,
	"Encoded_Library_Name": 59, "Encoded_Library_Version": 60, "Encoded_Library_Settings": 61, "Default": 62, "Forced": 63,
	"BufferSize": 64, "colour_description_present": 65, "colour_description_present_Source": 66, "colour_range": 67,
	"colour_range_Source": 68, "colour_primaries": 69, "colour_primaries_Source": 70, "transfer_characteristics": 71,
	"transfer_characteristics_Source": 72, "matrix_coefficients": 73, "matrix_coefficients_Source": 74,
	"List_StreamKind": 75, "List_StreamPos": 76, "ServiceName": 77, "ServiceProvider": 78, "ServiceType": 79, "extra": 80,
}

var jsonAudioFieldOrder = map[string]int{
	"@type": 0, "@typeorder": 1, "StreamOrder": 2, "FirstPacketOrder": 3, "ID": 4, "MenuID": 5, "UniqueID": 6,
	"Format": 7, "Format_Commercial_IfAny": 8, "Format_Settings_Endianness": 9, "Format_Version": 10,
	"Format_Settings_SBR": 11, "Format_AdditionalFeatures": 12, "MuxingMode": 13, "CodecID": 14, "Duration": 15,
	"Source_Duration": 16, "Source_Duration_LastFrame": 17, "BitRate_Mode": 18, "BitRate": 19, "BitRate_Maximum": 20,
	"Channels": 21, "ChannelPositions": 22, "ChannelLayout": 23, "SamplesPerFrame": 24, "SamplingRate": 25,
	"SamplingCount": 26, "FrameRate": 27, "FrameCount": 28, "Source_FrameCount": 29, "Compression_Mode": 30,
	"Delay": 31, "Delay_Source": 32, "Video_Delay": 33, "Encoded_Library": 34, "StreamSize": 35,
	"Source_StreamSize": 36, "Title": 37, "Language": 38, "ServiceKind": 39, "Default": 40, "Forced": 41,
	"AlternateGroup": 42, "extra": 43,
}

var jsonTextFieldOrder = map[string]int{
	"@type": 0, "@typeorder": 1, "StreamOrder": 2, "FirstPacketOrder": 3, "ID": 4, "UniqueID": 5,
	"Format": 6, "CodecID": 7, "MuxingMode_MoreInfo": 8, "Duration": 9, "BitDepth": 10,
	"Duration_Start2End": 11, "Duration_Start_Command": 12, "Duration_Start": 13, "Duration_End": 14, "Duration_End_Command": 15,
	"BitRate_Mode": 16, "BitRate": 17, "FrameRate": 18, "FrameCount": 19, "ElementCount": 20,
	"Delay": 21, "Video_Delay": 22, "StreamSize": 23, "FirstDisplay_Delay_Frames": 24, "FirstDisplay_Type": 25,
	"Title": 26, "Language": 27, "Default": 28, "Forced": 29, "extra": 30,
}

// jsonAVIGeneralFieldOrder and its stream-specific companions mirror the key
// order emitted by MediaInfo for AVI JSON.
var jsonAVIGeneralFieldOrder = makeJSONFieldOrder(
	"@type", "ID", "UniqueID", "VideoCount", "AudioCount", "TextCount", "ImageCount", "MenuCount", "FileExtension",
	"Format", "Format_Settings", "Interleaved", "FileSize", "Duration", "OverallBitRate_Mode", "OverallBitRate", "FrameRate", "FrameCount", "StreamSize",
	"File_Created_Date", "File_Created_Date_Local", "File_Modified_Date", "File_Modified_Date_Local",
	"Encoded_Application", "Encoded_Application_Name", "Encoded_Application_Version", "Encoded_Library", "extra",
)

var jsonAVIVideoFieldOrder = makeJSONFieldOrder(
	"@type", "@typeorder", "StreamOrder", "ID", "UniqueID", "Format", "Format_Version", "Format_Profile", "Format_Level",
	"Format_Settings_BVOP", "Format_Settings_QPel", "Format_Settings_GMC", "Format_Settings_Matrix", "MuxingMode", "CodecID",
	"Duration", "BitRate_Mode", "BitRate", "BitRate_Nominal", "BitRate_Maximum", "Width", "Height", "Sampled_Width", "Sampled_Height",
	"PixelAspectRatio", "DisplayAspectRatio", "FrameRate_Mode", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameCount",
	"ColorSpace", "ChromaSubsampling", "BitDepth", "ScanType", "Compression_Mode", "Delay", "StreamSize",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Date", "BufferSize", "extra",
)

var jsonAVIAudioFieldOrder = makeJSONFieldOrder(
	"@type", "@typeorder", "StreamOrder", "ID", "UniqueID", "Format", "Format_Version", "Format_Profile", "CodecID",
	"Duration", "BitRate_Mode", "BitRate", "Channels", "SamplingRate", "SamplingCount", "Compression_Mode", "Delay", "Delay_Source", "Video_Delay", "StreamSize",
	"Alignment", "Interleave_VideoFrames", "Interleave_Duration", "Interleave_Preload", "Title",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Date", "Language", "extra",
)

// jsonMatroskaGeneralFieldOrder and its stream-specific companions mirror the
// key order emitted by MediaInfo for Matroska JSON.
var jsonMatroskaGeneralFieldOrder = makeJSONFieldOrder(
	"@type", "ID", "UniqueID", "VideoCount", "AudioCount", "TextCount", "ImageCount", "MenuCount",
	"FileExtension", "CompleteName_Last", "Format", "Format_Version", "Format_Profile", "Format_Settings", "Interleaved", "CodecID", "CodecID_Compatible",
	"FileSize", "Duration", "OverallBitRate_Mode", "OverallBitRate", "FrameRate", "FrameCount", "StreamSize", "HeaderSize", "DataSize", "FooterSize", "IsStreamable",
	"Title", "Title_More", "Movie", "Movie_More", "Track_Position", "Track_Position_Total", "Performer", "EncodedBy", "Genre", "ContentType", "Synopsis", "Description",
	"Released_Date", "Recorded_Date", "Encoded_Date", "Tagged_Date", "Mastered_Date",
	"File_Created_Date", "File_Created_Date_Local", "File_Modified_Date", "File_Modified_Date_Local",
	"Encoded_Application", "Encoded_Application_Name", "Encoded_Application_Version",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Settings",
	"Copyright", "OriginalSourceForm", "BarCode", "TermsOfUse", "Cover", "Cover_Description", "Cover_Type", "Cover_Mime", "Comment", "extra",
)

var jsonMatroskaVideoFieldOrder = makeJSONFieldOrder(
	"@type", "@typeorder", "StreamOrder", "FirstPacketOrder", "ID", "MenuID", "OriginalSourceMedium_ID", "UniqueID",
	"Format", "Format_Version", "Format_Profile", "MultiView_Count", "Format_Level", "Format_Tier",
	"HDR_Format", "HDR_Format_Version", "HDR_Format_Profile", "HDR_Format_Level", "HDR_Format_Settings", "HDR_Format_Compression", "HDR_Format_Compatibility",
	"MultiView_Layout", "Format_Settings_BVOP", "Format_Settings_CABAC", "Format_Settings_QPel", "Format_Settings_RefFrames", "Format_Settings_GMC",
	"Format_Settings_Matrix", "Format_Settings_Matrix_Data", "Format_Settings_GOP", "Format_Settings_SliceCount", "Format_Settings_PictureStructure", "MuxingMode", "CodecID",
	"Duration", "BitRate_Mode", "BitRate", "BitRate_Nominal", "BitRate_Maximum",
	"Width", "Height", "Stored_Width", "Stored_Height", "Sampled_Width", "Sampled_Height", "PixelAspectRatio", "PixelAspectRatio_Original",
	"DisplayAspectRatio", "ActiveFormatDescription", "DisplayAspectRatio_Original", "Rotation",
	"FrameRate_Mode", "FrameRate_Mode_Original", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameRate_Original", "FrameCount", "Standard",
	"ColorSpace", "ChromaSubsampling", "ChromaSubsampling_Position", "BitDepth", "ScanType", "ScanOrder", "Compression_Mode",
	"Delay", "Delay_Settings", "Delay_DropFrame", "Delay_Source", "Delay_Original", "Delay_Original_DropFrame", "Delay_Original_Source",
	"TimeCode_FirstFrame", "TimeCode_Source", "Gop_OpenClosed", "Gop_OpenClosed_FirstFrame", "StreamSize", "Title",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Settings", "Language", "ServiceKind", "Encoded_Library_Date", "Default", "Forced", "BufferSize",
	"colour_description_present", "colour_description_present_Source", "colour_range", "colour_range_Source", "colour_primaries", "colour_primaries_Source", "colour_primaries_Original_Source",
	"transfer_characteristics", "transfer_characteristics_Source", "transfer_characteristics_Original", "transfer_characteristics_Original_Source",
	"matrix_coefficients", "matrix_coefficients_Source", "MasteringDisplay_ColorPrimaries", "MasteringDisplay_ColorPrimaries_Source",
	"MasteringDisplay_Luminance", "MasteringDisplay_Luminance_Source", "MasteringDisplay_Luminance_Min", "MasteringDisplay_Luminance_Max",
	"MaxCLL", "MaxCLL_Source", "MaxFALL", "matrix_coefficients_Original_Source", "MaxFALL_Source",
	"List_StreamKind", "List_StreamPos", "ServiceName", "ServiceProvider", "ServiceType", "extra",
)

var jsonMatroskaImageFieldOrder = makeJSONFieldOrder(
	"@type", "@typeorder", "StreamOrder", "Type", "Title", "Format", "Format_Profile", "Format_Compression", "Format_Settings_Packing", "MuxingMode",
	"Width", "Height", "PixelAspectRatio", "DisplayAspectRatio", "ColorSpace", "ChromaSubsampling", "BitDepth", "Compression_Mode", "StreamSize",
	"colour_description_present", "colour_range", "colour_primaries", "transfer_characteristics", "matrix_coefficients", "extra",
)

var jsonMatroskaAudioFieldOrder = makeJSONFieldOrder(
	"@type", "@typeorder", "StreamOrder", "FirstPacketOrder", "ID", "MenuID", "OriginalSourceMedium_ID", "UniqueID",
	"Format", "Format_Commercial_IfAny", "Format_Version", "Format_Settings_Floor", "Format_Profile", "Format_Settings_Mode", "Format_Settings_SBR", "Format_Settings_Endianness",
	"MuxingMode", "Format_Settings_Sign", "Format_Settings_ModeExtension", "Format_Settings_PS", "Format_AdditionalFeatures", "CodecID",
	"Duration", "Source_Duration", "Source_Duration_LastFrame", "BitRate_Mode", "BitRate", "BitRate_Minimum", "BitRate_Maximum",
	"Channels", "ChannelPositions", "Channels_Original", "ChannelLayout", "ChannelPositions_Original", "ChannelLayout_Original",
	"SamplesPerFrame", "SamplingRate", "SamplingCount", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameCount", "Source_FrameCount",
	"BitDepth", "BitDepth_Detected", "Compression_Mode", "Delay", "Delay_Source", "Video_Delay", "StreamSize", "Source_StreamSize",
	"Alignment", "Interleave_VideoFrames", "Interleave_Duration", "Title", "Encoded_Application", "Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version",
	"Encoded_Library_Settings", "Encoded_Library_Date", "Language", "ServiceKind", "Default", "Forced", "AlternateGroup", "Encoded_Date", "extra",
)

var jsonMatroskaTextFieldOrder = makeJSONFieldOrder(
	"@type", "@typeorder", "StreamOrder", "FirstPacketOrder", "ID", "MenuID", "OriginalSourceMedium_ID", "UniqueID", "Format", "MuxingMode", "CodecID", "MuxingMode_MoreInfo",
	"Duration", "BitDepth", "Duration_Start2End", "Duration_Start_Command", "Duration_Start", "Duration_End", "Duration_End_Command",
	"BitRate_Mode", "BitRate", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameCount", "ElementCount", "Compression_Mode", "Delay", "Video_Delay", "StreamSize",
	"FirstDisplay_Delay_Frames", "FirstDisplay_Type", "Title", "Encoded_Library", "Language", "ServiceKind", "Default", "Forced", "extra",
)

var jsonMenuFieldOrder = map[string]int{
	"@type":            0,
	"@typeorder":       1,
	"StreamOrder":      2,
	"FirstPacketOrder": 3,
	"ID":               4,
	"MenuID":           5,
	"Format":           6,
	"Duration":         7,
	"Delay":            8,
	"FrameRate":        9,
	"FrameRate_Num":    10,
	"FrameRate_Den":    11,
	"FrameCount":       12,
	"List_StreamKind":  13,
	"List_StreamPos":   14,
	"ServiceName":      15,
	"ServiceProvider":  16,
	"ServiceType":      17,
	"extra":            18,
}

// makeJSONFieldOrder converts an ordered key list into sortable positions.
func makeJSONFieldOrder(names ...string) map[string]int {
	order := make(map[string]int, len(names))
	for index, name := range names {
		order[name] = index
	}
	return order
}

func sortJSONFields(kind StreamKind, fields []jsonKV) []jsonKV {
	return sortJSONFieldsWithOrder(fields, jsonFieldOrder(kind))
}

// sortJSONFieldsForContainer reorders fields in place using a container-specific
// schema when one exists and the shared stream schema otherwise.
func sortJSONFieldsForContainer(kind StreamKind, fields []jsonKV, containerFormat string) []jsonKV {
	order := jsonFieldOrder(kind)
	switch containerFormat {
	case "Matroska":
		order = jsonMatroskaFieldOrder(kind)
	case "AVI":
		order = jsonAVIFieldOrder(kind)
	}
	return sortJSONFieldsWithOrder(fields, order)
}

// jsonAVIFieldOrder returns the AVI key order for kind.
func jsonAVIFieldOrder(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return jsonAVIGeneralFieldOrder
	case StreamVideo:
		return jsonAVIVideoFieldOrder
	case StreamAudio:
		return jsonAVIAudioFieldOrder
	case StreamText, StreamImage, StreamMenu:
		return jsonFieldOrder(kind)
	}
	panic("unreachable StreamKind")
}

// sortJSONFieldsWithOrder stably moves registered keys into schema order while
// retaining the relative order of unregistered dynamic keys.
func sortJSONFieldsWithOrder(fields []jsonKV, order map[string]int) []jsonKV {
	positions := map[string]int{}
	for i, field := range fields {
		positions[field.Key] = i
	}
	sort.SliceStable(fields, func(i, j int) bool {
		ai, aok := order[fields[i].Key]
		aj, bok := order[fields[j].Key]
		switch {
		case aok && bok:
			return ai < aj
		case aok:
			return true
		case bok:
			return false
		default:
			return positions[fields[i].Key] < positions[fields[j].Key]
		}
	})
	return fields
}

// jsonFieldOrder returns the shared JSON key order for kind.
func jsonFieldOrder(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return jsonGeneralFieldOrder
	case StreamAudio:
		return jsonAudioFieldOrder
	case StreamText:
		return jsonTextFieldOrder
	case StreamMenu:
		return jsonMenuFieldOrder
	case StreamImage:
		return jsonVideoFieldOrder
	case StreamVideo:
		return jsonVideoFieldOrder
	}
	return jsonVideoFieldOrder
}

// jsonMatroskaFieldOrder returns the Matroska key order for kind.
func jsonMatroskaFieldOrder(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return jsonMatroskaGeneralFieldOrder
	case StreamAudio:
		return jsonMatroskaAudioFieldOrder
	case StreamText:
		return jsonMatroskaTextFieldOrder
	case StreamImage:
		return jsonMatroskaImageFieldOrder
	case StreamVideo:
		return jsonMatroskaVideoFieldOrder
	case StreamMenu:
		return jsonMenuFieldOrder
	}
	return jsonMatroskaVideoFieldOrder
}

// isKnownJSONField reports whether name has a schema-backed position for kind.
// Structural object keys and the dynamic extra object are not data fields.
func isKnownJSONField(kind StreamKind, name string) bool {
	_, ok := jsonFieldOrder(kind)[name]
	if !ok {
		_, ok = jsonMatroskaFieldOrder(kind)[name]
	}
	return ok && name != "extra" && name != "@type" && name != "@typeorder"
}
