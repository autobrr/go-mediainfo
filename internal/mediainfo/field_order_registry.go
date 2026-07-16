package mediainfo

// textGeneralFieldOrder is the registered display order for General fields.
var textGeneralFieldOrder = map[string]int{
	"ID":                    -2,
	"Unique ID":             -1,
	"Complete name":         0,
	"CompleteName_Last":     0,
	"Format":                1,
	"Format/Info":           2,
	"Format settings":       3,
	"Format profile":        4,
	"Format version":        5,
	"Codec ID":              6,
	"File size":             7,
	"Duration":              8,
	"Overall bit rate mode": 9,
	"Overall bit rate":      10,
	"Frame rate":            11,
	"Writing application":   12,
	"Writing library":       13,
	"Encoded date":          14,
	"Tagged date":           15,
	"FileExtension_Invalid": 16,
	"Conformance warnings":  17,
	" General compliance":   18,
}

// textStreamFieldOrder is the registered display order shared by media fields.
var textStreamFieldOrder = map[string]int{
	"ID":                                0,
	"Menu ID":                           1,
	"Format":                            2,
	"Format/Info":                       3,
	"Commercial name":                   4,
	"Format version":                    4,
	"Format profile":                    5,
	"Muxing mode":                       6,
	"HDR format":                        6,
	"Format tier":                       6,
	"Format settings":                   7,
	"Format settings, BVOP":             8,
	"Format settings, QPel":             9,
	"Format settings, GMC":              10,
	"Format settings, Matrix":           11,
	"Format settings, GOP":              12,
	"Format settings, CABAC":            13,
	"Format settings, Reference frames": 14,
	"Format settings, Slice count":      14,
	"Codec ID":                          15,
	"Codec ID/Info":                     16,
	"Duration":                          17,
	"Source duration":                   18,
	"Source_Duration_LastFrame":         19,
	"Bit rate mode":                     20,
	"Bit rate":                          21,
	"Nominal bit rate":                  22,
	"Maximum bit rate":                  23,
	"Width":                             24,
	"Height":                            25,
	"Display aspect ratio":              26,
	"Channel(s)":                        27,
	"Channel layout":                    28,
	"Sampling rate":                     29,
	"Frame rate mode":                   30,
	"Frame rate":                        31,
	"Standard":                          32,
	"Color space":                       32,
	"Chroma subsampling":                33,
	"Chroma subsampling position":       33,
	"Bit depth":                         34,
	"Scan type":                         35,
	"Scan order":                        35,
	"Compression mode":                  36,
	"Bits/(Pixel*Frame)":                37,
	"Time code of first frame":          38,
	"Time code source":                  39,
	"GOP, Open/Closed":                  40,
	"GOP, Open/Closed of first frame":   41,
	"Delay relative to video":           42,
	"Count of elements":                 42,
	"Stream size":                       43,
	"Source stream size":                43,
	"Title":                             44,
	"Language":                          44,
	"Service kind":                      45,
	"Writing library":                   46,
	"Encoding settings":                 47,
	"Encoded date":                      48,
	"Tagged date":                       49,
	"Default":                           48,
	"Forced":                            49,
	"Complexity index":                  50,
	"Number of dynamic objects":         50,
	"Bed channel count":                 50,
	"Bed channel configuration":         50,
	"Dialog Normalization":              50,
	"bsid":                              50,
	"dsurmod":                           50,
	"acmod":                             50,
	"lfeon":                             50,
	"compr":                             50,
	"dynrng":                            50,
	"cmixlev":                           50,
	"surmixlev":                         50,
	"dmixmod":                           50,
	"ltrtcmixlev":                       50,
	"ltrtsurmixlev":                     50,
	"lorocmixlev":                       50,
	"lorosurmixlev":                     50,
	"mixlevel":                          50,
	"roomtyp":                           50,
	"dialnorm_Average":                  50,
	"dialnorm_Minimum":                  50,
	"dialnorm_Maximum":                  50,
	"dialnorm_Count":                    50,
	"compr_Average":                     50,
	"compr_Minimum":                     50,
	"compr_Maximum":                     50,
	"compr_Count":                       50,
	"dynrng_Average":                    50,
	"dynrng_Minimum":                    50,
	"dynrng_Maximum":                    50,
	"dynrng_Count":                      50,
	"Color range":                       51,
	"Color primaries":                   52,
	"Transfer characteristics":          53,
	"Matrix coefficients":               54,
	"Mastering display color primaries": 54,
	"Mastering display luminance":       54,
	"Maximum Content Light Level":       54,
	"Maximum Frame-Average Light Level": 54,
	"Alternate group":                   55,
	"Codec configuration box":           56,
	"List":                              57,
	"Service name":                      58,
	"Service provider":                  59,
	"Service type":                      60,
}

// textFieldOrderPolicy returns the registered display order for kind.
func textFieldOrderPolicy(kind StreamKind) map[string]int {
	if kind == StreamGeneral {
		return textGeneralFieldOrder
	}
	return textStreamFieldOrder
}

// structuredGeneralFieldOrder is the shared structured order for General keys.
var structuredGeneralFieldOrder = map[string]int{
	"@type": 0, "ID": 1, "UniqueID": 1, "VideoCount": 2, "AudioCount": 3, "TextCount": 4, "ImageCount": 5, "MenuCount": 6,
	"FileExtension": 7, "CompleteName_Last": 7, "Format": 8, "Format_Settings": 9, "Format_Version": 10, "Format_Profile": 11,
	"CodecID": 12, "CodecID_Compatible": 13, "FileSize": 14, "Duration": 15, "OverallBitRate_Mode": 16, "OverallBitRate": 17,
	"FrameRate": 18, "FrameCount": 19, "StreamSize": 20, "HeaderSize": 21, "DataSize": 22, "FooterSize": 23, "IsStreamable": 24,
	"File_Created_Date": 25, "File_Created_Date_Local": 26, "File_Modified_Date": 27, "File_Modified_Date_Local": 28,
	"Encoded_Application": 29, "Encoded_Application_Name": 30, "Encoded_Application_Version": 31,
	"Encoded_Library": 32, "Encoded_Library_Name": 33, "Encoded_Library_Version": 34, "Encoded_Library_Settings": 35, "extra": 36,
}

// structuredVideoFieldOrder is the shared structured order for video and image keys.
var structuredVideoFieldOrder = map[string]int{
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

// structuredAudioFieldOrder is the shared structured order for audio keys.
var structuredAudioFieldOrder = map[string]int{
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

// structuredTextFieldOrder is the shared structured order for text keys.
var structuredTextFieldOrder = map[string]int{
	"@type": 0, "@typeorder": 1, "StreamOrder": 2, "FirstPacketOrder": 3, "ID": 4, "UniqueID": 5,
	"Format": 6, "CodecID": 7, "MuxingMode_MoreInfo": 8, "Duration": 9, "BitDepth": 10,
	"Duration_Start2End": 11, "Duration_Start_Command": 12, "Duration_Start": 13, "Duration_End": 14, "Duration_End_Command": 15,
	"BitRate_Mode": 16, "BitRate": 17, "FrameRate": 18, "FrameCount": 19, "ElementCount": 20,
	"Delay": 21, "Video_Delay": 22, "StreamSize": 23, "FirstDisplay_Delay_Frames": 24, "FirstDisplay_Type": 25,
	"Title": 26, "Language": 27, "Default": 28, "Forced": 29, "extra": 30,
}

// structuredBDAVGeneralFieldOrder mirrors MediaInfo's BDAV General key order.
var structuredBDAVGeneralFieldOrder = makeStructuredFieldOrder(
	"@type", "ID", "UniqueID", "VideoCount", "AudioCount", "TextCount", "ImageCount", "MenuCount",
	"FileExtension", "CompleteName_Last", "Format", "Format_Settings", "Format_Version", "Format_Profile",
	"CodecID", "CodecID_Compatible", "FileSize", "Duration", "OverallBitRate_Mode", "OverallBitRate", "OverallBitRate_Maximum",
	"FrameRate", "FrameCount", "StreamSize", "HeaderSize", "DataSize", "FooterSize", "IsStreamable",
	"File_Created_Date", "File_Created_Date_Local", "File_Modified_Date", "File_Modified_Date_Local",
	"Encoded_Application", "Encoded_Application_Name", "Encoded_Application_Version",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Settings", "extra",
)

// structuredBDAVVideoFieldOrder mirrors MediaInfo's BDAV video key order while
// retaining Go-specific extensions at their semantic positions.
var structuredBDAVVideoFieldOrder = makeStructuredFieldOrder(
	"@type", "@typeorder", "StreamOrder", "FirstPacketOrder", "ID", "MenuID", "UniqueID",
	"Format", "Format_Version", "Format_Profile", "Format_Level", "Format_Tier",
	"HDR_Format", "HDR_Format_Version", "HDR_Format_Profile", "HDR_Format_Level", "HDR_Format_Settings", "HDR_Format_Compression", "HDR_Format_Compatibility",
	"Format_Settings_BVOP", "Format_Settings_CABAC", "Format_Settings_QPel", "Format_Settings_RefFrames", "Format_Settings_GMC",
	"Format_Settings_Matrix", "Format_Settings_Matrix_Data", "Format_Settings_GOP", "Format_Settings_SliceCount", "Format_Settings_PictureStructure",
	"MuxingMode", "CodecID", "Duration", "BitRate_Mode", "BitRate", "BitRate_Nominal", "BitRate_Maximum",
	"Width", "Height", "Stored_Width", "Stored_Height", "Sampled_Width", "Sampled_Height", "PixelAspectRatio", "DisplayAspectRatio", "Rotation",
	"FrameRate_Mode", "FrameRate_Mode_Original", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameCount", "Standard",
	"ColorSpace", "ChromaSubsampling", "ChromaSubsampling_Position", "BitDepth", "ScanType", "ScanOrder", "Compression_Mode",
	"Delay", "Delay_Settings", "Delay_DropFrame", "Delay_Source", "Delay_Original", "Delay_Original_DropFrame", "Delay_Original_Source",
	"TimeCode_FirstFrame", "TimeCode_Source", "Gop_OpenClosed", "Gop_OpenClosed_FirstFrame", "StreamSize",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Settings", "Default", "Forced", "BufferSize",
	"colour_description_present", "colour_description_present_Source", "colour_range", "colour_range_Source",
	"colour_primaries", "colour_primaries_Source", "transfer_characteristics", "transfer_characteristics_Source",
	"matrix_coefficients", "matrix_coefficients_Source", "MasteringDisplay_ColorPrimaries", "MasteringDisplay_ColorPrimaries_Source",
	"MasteringDisplay_Luminance", "MasteringDisplay_Luminance_Source", "MasteringDisplay_Luminance_Min", "MasteringDisplay_Luminance_Max",
	"MaxCLL", "MaxCLL_Source", "MaxFALL", "MaxFALL_Source", "extra",
)

// structuredBDAVAudioFieldOrder mirrors MediaInfo's BDAV audio key order while
// keeping encoded-rate and encoded-size extensions adjacent to their base keys.
var structuredBDAVAudioFieldOrder = makeStructuredFieldOrder(
	"@type", "@typeorder", "StreamOrder", "FirstPacketOrder", "ID", "MenuID", "UniqueID",
	"Format", "Format_Commercial_IfAny", "Format_Version", "Format_Profile", "Format_Settings_Mode", "Format_Settings_Endianness",
	"Format_Settings_Sign", "Format_AdditionalFeatures", "MuxingMode", "CodecID",
	"Duration", "Source_Duration", "Source_Duration_LastFrame", "BitRate_Mode", "BitRate", "BitRate_Encoded", "BitRate_Minimum", "BitRate_Maximum",
	"Channels", "ChannelPositions", "Channels_Original", "ChannelLayout", "ChannelPositions_Original", "ChannelLayout_Original",
	"SamplesPerFrame", "SamplingRate", "SamplingCount", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameCount", "Source_FrameCount",
	"BitDepth", "BitDepth_Detected", "Compression_Mode", "Delay", "Delay_Source", "Video_Delay", "StreamSize", "StreamSize_Encoded", "Source_StreamSize",
	"Alignment", "Interleave_VideoFrames", "Interleave_Duration", "Title", "Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version",
	"Encoded_Library_Settings", "Language", "ServiceKind", "Default", "Forced", "AlternateGroup", "extra",
)

// structuredBDAVTextFieldOrder keeps Blu-ray subtitle identity fields before
// Go-specific timing extensions.
var structuredBDAVTextFieldOrder = makeStructuredFieldOrder(
	"@type", "@typeorder", "StreamOrder", "FirstPacketOrder", "ID", "MenuID", "UniqueID",
	"Format", "CodecID", "MuxingMode_MoreInfo", "Duration", "BitDepth", "Duration_Start2End", "Duration_Start_Command",
	"Duration_Start", "Duration_End", "Duration_End_Command", "BitRate_Mode", "BitRate", "FrameRate", "FrameCount", "ElementCount",
	"Delay", "Delay_Source", "Video_Delay", "StreamSize", "FirstDisplay_Delay_Frames", "FirstDisplay_Type",
	"Title", "Language", "Default", "Forced", "extra",
)

// structuredAVIGeneralFieldOrder and its stream-specific companions mirror the key
// order emitted by MediaInfo for AVI structured output.
var structuredAVIGeneralFieldOrder = makeStructuredFieldOrder(
	"@type", "ID", "UniqueID", "VideoCount", "AudioCount", "TextCount", "ImageCount", "MenuCount", "FileExtension",
	"Format", "Format_Settings", "Interleaved", "FileSize", "Duration", "OverallBitRate_Mode", "OverallBitRate", "FrameRate", "FrameCount", "StreamSize",
	"File_Created_Date", "File_Created_Date_Local", "File_Modified_Date", "File_Modified_Date_Local",
	"Encoded_Application", "Encoded_Application_Name", "Encoded_Application_Version", "Encoded_Library", "extra",
)

// structuredAVIVideoFieldOrder is the evidenced AVI override for video keys.
var structuredAVIVideoFieldOrder = makeStructuredFieldOrder(
	"@type", "@typeorder", "StreamOrder", "ID", "UniqueID", "Format", "Format_Version", "Format_Profile", "Format_Level",
	"Format_Settings_BVOP", "Format_Settings_QPel", "Format_Settings_GMC", "Format_Settings_Matrix", "MuxingMode", "CodecID",
	"Duration", "BitRate_Mode", "BitRate", "BitRate_Nominal", "BitRate_Maximum", "Width", "Height", "Sampled_Width", "Sampled_Height",
	"PixelAspectRatio", "DisplayAspectRatio", "FrameRate_Mode", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameCount",
	"ColorSpace", "ChromaSubsampling", "BitDepth", "ScanType", "Compression_Mode", "Delay", "StreamSize",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Date", "BufferSize", "extra",
)

// structuredAVIAudioFieldOrder is the evidenced AVI override for audio keys.
var structuredAVIAudioFieldOrder = makeStructuredFieldOrder(
	"@type", "@typeorder", "StreamOrder", "ID", "UniqueID", "Format", "Format_Version", "Format_Profile", "CodecID",
	"Duration", "BitRate_Mode", "BitRate", "Channels", "SamplingRate", "SamplingCount", "Compression_Mode", "Delay", "Delay_Source", "Video_Delay", "StreamSize",
	"Alignment", "Interleave_VideoFrames", "Interleave_Duration", "Interleave_Preload", "Title",
	"Encoded_Library", "Encoded_Library_Name", "Encoded_Library_Version", "Encoded_Library_Date", "Language", "extra",
)

// structuredMatroskaGeneralFieldOrder and its stream-specific companions mirror the
// key order emitted by MediaInfo for Matroska structured output.
var structuredMatroskaGeneralFieldOrder = makeStructuredFieldOrder(
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

// structuredMatroskaVideoFieldOrder is the evidenced Matroska override for video keys.
var structuredMatroskaVideoFieldOrder = makeStructuredFieldOrder(
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

// structuredMatroskaImageFieldOrder is the evidenced Matroska override for image keys.
var structuredMatroskaImageFieldOrder = makeStructuredFieldOrder(
	"@type", "@typeorder", "StreamOrder", "Type", "Title", "Format", "Format_Profile", "Format_Compression", "Format_Settings_Packing", "MuxingMode",
	"Width", "Height", "PixelAspectRatio", "DisplayAspectRatio", "ColorSpace", "ChromaSubsampling", "BitDepth", "Compression_Mode", "StreamSize",
	"colour_description_present", "colour_range", "colour_primaries", "transfer_characteristics", "matrix_coefficients", "extra",
)

// structuredMatroskaAudioFieldOrder is the evidenced Matroska override for audio keys.
var structuredMatroskaAudioFieldOrder = makeStructuredFieldOrder(
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

// structuredMatroskaTextFieldOrder is the evidenced Matroska override for text keys.
var structuredMatroskaTextFieldOrder = makeStructuredFieldOrder(
	"@type", "@typeorder", "StreamOrder", "FirstPacketOrder", "ID", "MenuID", "OriginalSourceMedium_ID", "UniqueID", "Format", "MuxingMode", "CodecID", "MuxingMode_MoreInfo",
	"Duration", "BitDepth", "Duration_Start2End", "Duration_Start_Command", "Duration_Start", "Duration_End", "Duration_End_Command",
	"BitRate_Mode", "BitRate", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameCount", "ElementCount", "Compression_Mode", "Delay", "Video_Delay", "StreamSize",
	"FirstDisplay_Delay_Frames", "FirstDisplay_Type", "Title", "Encoded_Library", "Language", "ServiceKind", "Default", "Forced", "extra",
)

// structuredMenuFieldOrder is the shared structured order for menu keys.
var structuredMenuFieldOrder = map[string]int{
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

// makeStructuredFieldOrder converts an ordered key list into sortable positions.
func makeStructuredFieldOrder(names ...string) map[string]int {
	order := make(map[string]int, len(names))
	for index, name := range names {
		order[name] = index
	}
	return order
}

// structuredFieldOrderForContainer returns the active structured order policy.
func structuredFieldOrderForContainer(kind StreamKind, containerFormat string) map[string]int {
	switch containerFormat {
	case "Matroska":
		return structuredMatroskaFieldOrderPolicy(kind)
	case "AVI":
		return structuredAVIFieldOrderPolicy(kind)
	case "BDAV":
		return structuredBDAVFieldOrderPolicy(kind)
	default:
		return structuredFieldOrderPolicy(kind)
	}
}

// structuredBDAVFieldOrderPolicy returns the BDAV key order for kind.
func structuredBDAVFieldOrderPolicy(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return structuredBDAVGeneralFieldOrder
	case StreamVideo, StreamImage:
		return structuredBDAVVideoFieldOrder
	case StreamAudio:
		return structuredBDAVAudioFieldOrder
	case StreamText:
		return structuredBDAVTextFieldOrder
	case StreamMenu:
		return structuredMenuFieldOrder
	}
	panic("unreachable StreamKind")
}

// structuredAVIFieldOrderPolicy returns the AVI key order for kind.
func structuredAVIFieldOrderPolicy(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return structuredAVIGeneralFieldOrder
	case StreamVideo:
		return structuredAVIVideoFieldOrder
	case StreamAudio:
		return structuredAVIAudioFieldOrder
	case StreamText, StreamImage, StreamMenu:
		return structuredFieldOrderPolicy(kind)
	}
	panic("unreachable StreamKind")
}

// structuredFieldOrderPolicy returns the shared structured key order for kind.
func structuredFieldOrderPolicy(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return structuredGeneralFieldOrder
	case StreamAudio:
		return structuredAudioFieldOrder
	case StreamText:
		return structuredTextFieldOrder
	case StreamMenu:
		return structuredMenuFieldOrder
	case StreamImage:
		return structuredVideoFieldOrder
	case StreamVideo:
		return structuredVideoFieldOrder
	}
	return structuredVideoFieldOrder
}

// structuredMatroskaFieldOrderPolicy returns the Matroska key order for kind.
func structuredMatroskaFieldOrderPolicy(kind StreamKind) map[string]int {
	switch kind {
	case StreamGeneral:
		return structuredMatroskaGeneralFieldOrder
	case StreamAudio:
		return structuredMatroskaAudioFieldOrder
	case StreamText:
		return structuredMatroskaTextFieldOrder
	case StreamImage:
		return structuredMatroskaImageFieldOrder
	case StreamVideo:
		return structuredMatroskaVideoFieldOrder
	case StreamMenu:
		return structuredMenuFieldOrder
	}
	return structuredMatroskaVideoFieldOrder
}

// isKnownStructuredField reports whether name has a schema-backed position for kind.
// Structural object keys and the dynamic extra object are not data fields.
func isKnownStructuredField(kind StreamKind, name string) bool {
	_, ok := structuredFieldOrderPolicy(kind)[name]
	if !ok {
		_, ok = structuredMatroskaFieldOrderPolicy(kind)[name]
	}
	return ok && name != "extra" && name != "@type" && name != "@typeorder"
}
