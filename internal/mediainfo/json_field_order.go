package mediainfo

import "sort"

var jsonGeneralFieldOrder = map[string]int{
	"@type":                    0,
	"ID":                       1,
	"UniqueID":                 1,
	"VideoCount":               2,
	"AudioCount":               3,
	"TextCount":                4,
	"ImageCount":               5,
	"MenuCount":                6,
	"FileExtension":            7,
	"CompleteName_Last":        7,
	"Format":                   8,
	"Format_Settings":          9,
	"Format_Version":           10,
	"Format_Profile":           11,
	"CodecID":                  12,
	"CodecID_Compatible":       13,
	"FileSize":                 14,
	"Duration":                 15,
	"OverallBitRate_Mode":      16,
	"OverallBitRate":           17,
	"FrameRate":                18,
	"FrameCount":               19,
	"StreamSize":               20,
	"HeaderSize":               21,
	"DataSize":                 22,
	"FooterSize":               23,
	"IsStreamable":             24,
	"File_Created_Date":        25,
	"File_Created_Date_Local":  26,
	"File_Modified_Date":       27,
	"File_Modified_Date_Local": 28,
	"Encoded_Application":      29,
	"Encoded_Library":          30,
	"Encoded_Library_Name":     31,
	"Encoded_Library_Version":  32,
	"Encoded_Library_Settings": 33,
	"extra":                    34,
}

var jsonVideoFieldOrder = map[string]int{
	"@type":                             0,
	"@typeorder":                        1,
	"StreamOrder":                       2,
	"FirstPacketOrder":                  3,
	"ID":                                4,
	"MenuID":                            5,
	"UniqueID":                          6,
	"Format":                            7,
	"Format_Version":                    8,
	"Format_Profile":                    9,
	"Format_Level":                      10,
	"Format_Settings_CABAC":             11,
	"Format_Settings_RefFrames":         12,
	"Format_Settings_BVOP":              13,
	"Format_Settings_QPel":              13,
	"Format_Settings_GMC":               13,
	"Format_Settings_Matrix":            14,
	"Format_Settings_Matrix_Data":       15,
	"Format_Settings_GOP":               16,
	"CodecID":                           17,
	"Duration":                          18,
	"BitRate_Mode":                      19,
	"BitRate":                           20,
	"BitRate_Nominal":                   21,
	"BitRate_Maximum":                   22,
	"Width":                             23,
	"Height":                            24,
	"Stored_Height":                     25,
	"Sampled_Width":                     26,
	"Sampled_Height":                    27,
	"PixelAspectRatio":                  28,
	"DisplayAspectRatio":                29,
	"Rotation":                          30,
	"FrameRate_Mode":                    31,
	"FrameRate_Mode_Original":           32,
	"FrameRate":                         33,
	"FrameRate_Num":                     34,
	"FrameRate_Den":                     35,
	"FrameCount":                        36,
	"Standard":                          37,
	"ColorSpace":                        38,
	"ChromaSubsampling":                 39,
	"BitDepth":                          40,
	"ScanType":                          41,
	"Compression_Mode":                  42,
	"Delay":                             43,
	"Delay_Settings":                    44,
	"Delay_DropFrame":                   45,
	"Delay_Source":                      46,
	"Delay_Original":                    47,
	"Delay_Original_DropFrame":          48,
	"Delay_Original_Source":             49,
	"TimeCode_FirstFrame":               50,
	"TimeCode_Source":                   51,
	"Gop_OpenClosed":                    52,
	"Gop_OpenClosed_FirstFrame":         53,
	"StreamSize":                        54,
	"Encoded_Library":                   56,
	"Encoded_Library_Name":              57,
	"Encoded_Library_Version":           58,
	"Encoded_Library_Settings":          59,
	"Default":                           60,
	"Forced":                            61,
	"BufferSize":                        62,
	"colour_description_present":        63,
	"colour_description_present_Source": 64,
	"colour_range":                      65,
	"colour_range_Source":               66,
	"colour_primaries":                  67,
	"colour_primaries_Source":           68,
	"transfer_characteristics":          69,
	"transfer_characteristics_Source":   70,
	"matrix_coefficients":               71,
	"matrix_coefficients_Source":        72,
	"List_StreamKind":                   73,
	"List_StreamPos":                    74,
	"ServiceName":                       75,
	"ServiceProvider":                   76,
	"ServiceType":                       77,
	"extra":                             78,
}

var jsonAudioFieldOrder = map[string]int{
	"@type":                      0,
	"@typeorder":                 1,
	"StreamOrder":                2,
	"FirstPacketOrder":           3,
	"ID":                         4,
	"MenuID":                     5,
	"UniqueID":                   6,
	"Format":                     7,
	"Format_Commercial_IfAny":    8,
	"Format_Settings_Endianness": 9,
	"Format_Version":             10,
	"Format_Settings_SBR":        11,
	"Format_AdditionalFeatures":  12,
	"MuxingMode":                 13,
	"CodecID":                    14,
	"Duration":                   15,
	"Source_Duration":            16,
	"Source_Duration_LastFrame":  17,
	"BitRate_Mode":               18,
	"BitRate":                    19,
	"BitRate_Maximum":            20,
	"Channels":                   21,
	"ChannelPositions":           22,
	"ChannelLayout":              23,
	"SamplesPerFrame":            24,
	"SamplingRate":               25,
	"SamplingCount":              26,
	"FrameRate":                  27,
	"FrameCount":                 28,
	"Source_FrameCount":          29,
	"Compression_Mode":           30,
	"Delay":                      31,
	"Delay_Source":               32,
	"Video_Delay":                33,
	"Encoded_Library":            34,
	"StreamSize":                 35,
	"Source_StreamSize":          36,
	"Language":                   37,
	"ServiceKind":                38,
	"Default":                    39,
	"Forced":                     40,
	"AlternateGroup":             41,
	"extra":                      42,
}

var jsonTextFieldOrder = map[string]int{
	"@type":                     0,
	"@typeorder":                1,
	"StreamOrder":               2,
	"FirstPacketOrder":          3,
	"ID":                        4,
	"UniqueID":                  5,
	"Format":                    6,
	"CodecID":                   7,
	"MuxingMode_MoreInfo":       8,
	"Duration":                  9,
	"BitDepth":                  10,
	"Duration_Start2End":        11,
	"Duration_Start_Command":    12,
	"Duration_Start":            13,
	"Duration_End":              14,
	"Duration_End_Command":      15,
	"BitRate_Mode":              16,
	"BitRate":                   17,
	"FrameRate":                 18,
	"FrameCount":                19,
	"ElementCount":              20,
	"Delay":                     21,
	"Video_Delay":               22,
	"StreamSize":                23,
	"FirstDisplay_Delay_Frames": 24,
	"FirstDisplay_Type":         25,
	"Title":                     26,
	"Language":                  27,
	"Default":                   28,
	"Forced":                    29,
	"extra":                     30,
}

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

func sortJSONFields(kind StreamKind, fields []jsonKV) []jsonKV {
	order := jsonVideoFieldOrder
	switch kind {
	case StreamGeneral:
		order = jsonGeneralFieldOrder
	case StreamAudio:
		order = jsonAudioFieldOrder
	case StreamVideo:
		order = jsonVideoFieldOrder
	case StreamText:
		order = jsonTextFieldOrder
	case StreamMenu:
		order = jsonMenuFieldOrder
	case StreamImage:
		order = jsonVideoFieldOrder
	}
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
