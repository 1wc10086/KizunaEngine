//---------------------------------------------------------------------------
/*
        TVP2 ( T Visual Presenter 2 )  A script authoring tool
        Copyright (C) 2000 W.Dee <dee@kikyou.info> and contributors

        See details of license at "license.txt"
*/
//---------------------------------------------------------------------------
// Wave Player interface
//---------------------------------------------------------------------------
#include "tjsCommHead.h"

#include "TVPWaveManager.h"
#include "TVPMsg.h"
#include "UtilStreams.h"

#include "RIFFWaveDecoder.h"
#include "HostSilentWaveDecoder.h"

//---------------------------------------------------------------------------
// tTVPWaveDecoder interface management
//---------------------------------------------------------------------------
static bool TVPWaveDecoderManagerAvail = false;
struct tTVPWaveDecoderManager
{
    std::vector<tTVPWaveDecoderCreator*> Creators;
    tTVPWDC_RIFFWave RIFFWaveDecoderCreator;
    tTVPWDC_HostSilent HostSilentWaveDecoderCreator;

    tTVPWaveDecoderManager()
    {
        TVPWaveDecoderManagerAvail = true;
        TVPRegisterWaveDecoderCreator(&RIFFWaveDecoderCreator);
        // Host-delegated silent fallback (see HostSilentWaveDecoder.h): right
        // after the RIFF decoder is the lowest priority slot because
        // TVPCreateWaveDecoder walks Creators from the back down to index 1, so
        // every decoder a plugin registers later is consulted before this one,
        // and the RIFF decoder is still consulted first.
        TVPRegisterWaveDecoderCreator(&HostSilentWaveDecoderCreator);
    }

    ~tTVPWaveDecoderManager() { TVPWaveDecoderManagerAvail = false; }

} static TVPWaveDecoderManager;
//---------------------------------------------------------------------------
void TVPRegisterWaveDecoderCreator(tTVPWaveDecoderCreator* d)
{
    if (TVPWaveDecoderManagerAvail)
        TVPWaveDecoderManager.Creators.push_back(d);
}
//---------------------------------------------------------------------------
void TVPUnregisterWaveDecoderCreator(tTVPWaveDecoderCreator* d)
{
    if (TVPWaveDecoderManagerAvail)
    {
        std::vector<tTVPWaveDecoderCreator*>::iterator i;
        i = std::find(TVPWaveDecoderManager.Creators.begin(), TVPWaveDecoderManager.Creators.end(),
                      d);
        if (i != TVPWaveDecoderManager.Creators.end())
        {
            TVPWaveDecoderManager.Creators.erase(i);
        }
    }
}
//---------------------------------------------------------------------------
tTVPWaveDecoder* TVPCreateWaveDecoder(const ttstr& storagename)
{
    // find a decoder and create its instance.
    // throws an exception when the decodable decoder is not found.
    if (!TVPWaveDecoderManagerAvail)
        return NULL;

    ttstr ext(TVPExtractStorageExt(storagename));
    ext.ToLowerCase();

    // .wav first
    tTVPWaveDecoder* decoder = TVPWaveDecoderManager.Creators[0]->Create(storagename, ext);
    if (decoder)
        return decoder;

    // other
    tjs_int i = (tjs_int)(TVPWaveDecoderManager.Creators.size() - 1);
    for (; i >= 1; i--)
    {
        decoder = TVPWaveDecoderManager.Creators[i]->Create(storagename, ext);
        if (decoder)
            return decoder;
    }

    TVPThrowExceptionMessage(TVPUnknownWaveFormat, storagename);
    return NULL;
}
//---------------------------------------------------------------------------