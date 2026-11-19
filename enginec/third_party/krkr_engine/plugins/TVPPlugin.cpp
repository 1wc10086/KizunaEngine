//---------------------------------------------------------------------------
/*
        TVP2 ( T Visual Presenter 2 )  A script authoring tool
        Copyright (C) 2000 W.Dee <dee@kikyou.info> and contributors

        See details of license at "license.txt"
*/
//---------------------------------------------------------------------------
// "Plugins" class implementation / Service for plug-ins
//---------------------------------------------------------------------------
#include "tjsCommHead.h"

#include <algorithm>
#include <functional>
#include "TVPScript.h"
#include "TVPPlugin.h"
#include "TVPStorage.h"
#include "TVPGraphicsLoader.h"

#include "TVPMsg.h"
#include "TVPSystem.h"

#include "tjsHashSearch.h"
#include "TVPEvent.h"
#include "TVPTrans.h"
#include "tjsArray.h"
#include "tjsDictionary.h"
#include "TVPDebug.h"
#include "PlatformFile.h"
#include "tjs.h"
#include "FuncStubs.h"

#include "TVPApplication.h"

//---------------------------------------------------------------------------
bool TVPLoadInternalPlugin(const ttstr& _name);
extern std::set<ttstr> TVPRegisteredPlugins;
static bool TVPPluginLoading = false;
void TVPLoadPlugin(const ttstr& name)
{
    // Plugins are sealed into the binary (no runtime dlopen on Android), so a
    // link either finds a compiled-in module or does nothing. Recording the
    // miss here is the difference between "the game links layerExDraw.dll and
    // gets GdiPlus" and a bare "Member GdiPlus does not exist" much later.
    if (TVPLoadInternalPlugin(name))
        return;
    // LoadModule() also answers false for a module that is already registered;
    // preloaded modules (layerExDraw, sqlite3, ...) are linked by nearly every
    // game, and warning about those would drown the real misses.
    ttstr normalized = TVPExtractStorageName(name);
    normalized.ToLowerCase();
    if (TVPRegisteredPlugins.find(normalized) != TVPRegisteredPlugins.end())
        return;
    TVPAddLog(ttstr(TJS_N("(warn) Plugins.link: no such plugin: ")) + name);
}
//---------------------------------------------------------------------------
bool TVPUnloadPlugin(const ttstr& name)
{
    return true;
}
//---------------------------------------------------------------------------

//---------------------------------------------------------------------------
// plug-in autoload support
//---------------------------------------------------------------------------
struct tTVPFoundPlugin
{
    std::string Path;
    std::string Name;
    bool operator<(const tTVPFoundPlugin& rhs) const { return Name < rhs.Name; }
};
static tjs_int TVPAutoLoadPluginCount = 0;
static void TVPSearchPluginsAt(std::vector<tTVPFoundPlugin>& list, const ttstr& folder)
{
    std::string folderName = TVPGetLocallyAccessibleName(folder).AsStdString();
    TVPListDir(folderName,
               [&](const std::string& filename, int mask)
               {
                   if (mask & S_IFREG)
                   {
                       if (!TJS_strcasecmp(filename.c_str() + filename.length() - 4, ".tpm"))
                       {
                           tTVPFoundPlugin fp;
                           fp.Path = folderName;
                           fp.Name = filename;
                           list.emplace_back(fp);
                       }
                   }
               });
}

void TVPLoadInternalPlugins();
void TVPLoadPluigins(void)
{
    TVPLoadInternalPlugins();
    // This function searches plugins which have an extension of ".tpm"
    // in the default path:
    //    1. a folder which holds kirikiri executable
    //    2. "plugin" folder of it
    // Plugin load order is to be decided using its name;
    // aaa.tpm is to be loaded before aab.tpm (sorted by ASCII order)

    // search plugins from path: (exepath), (exepath)\system, (exepath)\plugin
    std::vector<tTVPFoundPlugin> list;

    TVPSearchPluginsAt(list, TVPProjectDir);
    TVPSearchPluginsAt(list, TVPProjectDir + "system");
    TVPSearchPluginsAt(list, TVPProjectDir + "plugin");

    // sort by filename
    std::sort(list.begin(), list.end());

    // load each plugin
    TVPAutoLoadPluginCount = (tjs_int)list.size();
    for (std::vector<tTVPFoundPlugin>::iterator i = list.begin(); i != list.end(); i++)
    {
        TVPAddImportantLog(ttstr(TJS_N("(info) Loading ")) + ttstr(i->Name.c_str()));
        TVPLoadPlugin((i->Path + "/" + i->Name).c_str());
    }
}
//---------------------------------------------------------------------------
void TVPUnloadInternalPlugins();
void TVPUnloadPlugins()
{
    TVPUnloadInternalPlugins();
}
//---------------------------------------------------------------------------
tjs_int TVPGetAutoLoadPluginCount()
{
    return TVPAutoLoadPluginCount;
}
//---------------------------------------------------------------------------

//---------------------------------------------------------------------------
bool TVPRegisterGlobalObject(const tjs_char* name, iTJSDispatch2* dsp)
{
    // register given object to global object
    tTJSVariant val(dsp);
    iTJSDispatch2* global = TVPGetScriptDispatch();
    tjs_error er;
    try
    {
        er = global->PropSet(TJS_MEMBERENSURE, name, NULL, &val, global);
    }
    catch (...)
    {
        global->Release();
        return false;
    }
    global->Release();
    return TJS_SUCCEEDED(er);
}
//---------------------------------------------------------------------------
bool TVPRemoveGlobalObject(const tjs_char* name)
{
    // remove registration of global object
    iTJSDispatch2* global = TVPGetScriptDispatch();
    if (!global)
        return false;
    tjs_error er;
    try
    {
        er = global->DeleteMember(0, name, NULL, global);
    }
    catch (...)
    {
        global->Release();
        return false;
    }
    global->Release();
    return TJS_SUCCEEDED(er);
}
//---------------------------------------------------------------------------
void TVPDoTryBlock(tTVPTryBlockFunction tryblock,
                   tTVPCatchBlockFunction catchblock,
                   tTVPFinallyBlockFunction finallyblock,
                   void* data)
{
    try
    {
        tryblock(data);
    }
    catch (const eTJS& e)
    {
        if (finallyblock)
            finallyblock(data);
        tTVPExceptionDesc desc;
        desc.type = TJS_N("eTJS");
        desc.message = e.GetMessage();
        if (catchblock(data, desc))
            throw;
        return;
    }
    catch (...)
    {
        if (finallyblock)
            finallyblock(data);
        tTVPExceptionDesc desc;
        desc.type = TJS_N("unknown");
        if (catchblock(data, desc))
            throw;
        return;
    }
    if (finallyblock)
        finallyblock(data);
}
//---------------------------------------------------------------------------

#include "ncbind/ncbind.hpp"

// Retail ships a few DLLs whose script surface this build produces from another
// place: the class is a core native (menu.dll, KAGParser.dll, layerExSave.dll),
// the code lives in another module (gfxFire.dll is compiled into gfxEffect.dll),
// or the work is done by the host (the audio decoders). Linking one of them must
// still succeed: KAG3's Initialize.tjs and per-title system scripts branch on
// Plugins.getList() and on the link result, and a "missing plugin" answer sends
// them down a fallback path the retail build never takes — Aether's
// PolyfillInitialize.tjs is exactly such a fallback (it installs a pure-TJS
// MenuItem/KAGParser when the link fails).
//
// `provided_by` names the compiled module that must be loaded first; nullptr
// means the surface is a core class registered before any script runs, so the
// link only has to be recorded.
namespace
{
struct tTVPPluginAlias
{
    const tjs_char* name;
    const tjs_char* provided_by;
};

constexpr tTVPPluginAlias TVPPluginAliases[] = {
    // MenuItem is a core class (core/script/TVPScript.cpp); retail also ships it
    // as a separate DLL for engines that do not build it in.
    {TJS_N("menu.dll"), nullptr},
    // KAGParser is a core class too; ExtKAGParser.dll (preloaded) has already
    // replaced it with the KAG3 implementation by the time a game links this.
    {TJS_N("KAGParser.dll"), nullptr},
    // Layer.saveLayerImage/loadImages are core methods. Upstream's
    // LayerExSave.cpp is an empty placeholder, so there is no module to build;
    // the save surface a game reaches for already exists.
    {TJS_N("layerExSave.dll"), nullptr},
    // gfxFire.cpp is compiled into the gfxEffect.dll module (upstream gives it
    // the same NCB module name), so the retail gfxFire.dll name loads that.
    {TJS_N("gfxFire.dll"), TJS_N("gfxEffect.dll")},
    // The audio decoders are host-delegated: every storage reaches the host
    // through environ/PlatformAudioHook.h and HostSilentWaveDecoder keeps the
    // engine-side timeline, so there is no C++ decoder to register. The module
    // names stay linkable to keep the plugin set scripts observe identical.
    {TJS_N("wuffmpeg.dll"), nullptr},
    {TJS_N("wuopus.dll"), nullptr},
    {TJS_N("wuvorbis.dll"), nullptr},
};

// Records `name` as linked when an alias satisfies it. Unloading never undoes an
// alias: the surface behind it is a core class or the host, neither of which the
// plugin manager owns.
bool TVPLoadAliasedPlugin(const ttstr& name)
{
    for (const auto& alias : TVPPluginAliases)
    {
        if (TJS_strcasecmp(name.c_str(), alias.name) != 0)
            continue;
        if (alias.provided_by)
        {
            ttstr target(alias.provided_by);
            target.ToLowerCase();
            if (TVPRegisteredPlugins.find(target) == TVPRegisteredPlugins.end() &&
                !ncbAutoRegister::LoadModule(alias.provided_by))
                return false;
        }
        ttstr key(name);
        key.ToLowerCase();
        TVPRegisteredPlugins.insert(key);
        return true;
    }
    return false;
}
} // namespace

void TVPLoadInternalPlugins()
{
    // Modules the engine makes available before the game's own scripts run.
    //
    // kirikiroid2/xp3filter/wuffmpeg/wuopus/wuvorbis/win32dialog/varfile/
    // shrinkCopy are upstream's preload list. The rest are preloaded because
    // their names are reached directly at startup by the system scripts:
    // extkagparser swaps the core KAGParser for the implementation KAG3 expects
    // (macros, labels); saveStruct/textrender/addFont/json cover save
    // serialization, text layout, dynamic fonts and JSON; layerExDraw is the
    // single most important one because its `GdiPlus` namespace and
    // `Layer.clear()`/`drawString()` members are used while KAG builds its
    // message frame, and a missing member there aborts the whole script (the
    // observed symptom is a title screen that ignores every click).
    //
    // Deliberately NOT preloaded:
    //   * KAGParserEx: it replaces the global KAGParser class on load, which
    //     would change parsing for every older KAG3 title. Games that want it
    //     call Plugins.link("KAGParserEx.dll") themselves.
    //   * drawdeviceD3D: games probe `typeof D3D`/`DrawDeviceD3D` to decide
    //     whether a GPU device exists. Preloading it would make that probe lie,
    //     so it is only linked on demand.
    //   * wuopus/wuvorbis/wuffmpeg: decoders live in the host, but retail loads
    //     those modules at startup and scripts do see them in Plugins.getList(),
    //     so the alias table satisfies the names here.
    static const struct
    {
        const tjs_char* name;
        bool built;
    } modules[] = {
        {TJS_N("kirikiroid2.dll"), true},  // JPEG/PNG/WebP loaders, text encoding
        {TJS_N("extkagparser.dll"), true}, // KAGParser replacement (macros, labels)
        {TJS_N("xp3filter.dll"), true},    // xp3filter.tjs content/extraction filter
        {TJS_N("win32dialog.dll"), true},  // System.inform-backed message box
        {TJS_N("varfile.dll"), true},      // in-memory virtual files
        {TJS_N("shrinkCopy.dll"), true},   // fast layer blends
        {TJS_N("saveStruct.dll"), true},   // Array/Dictionary save-string helpers
        {TJS_N("textrender.dll"), true},   // TextRender engine used by KAG message layers
        {TJS_N("addFont.dll"), true},      // System.addFont private fonts
        {TJS_N("json.dll"), true},         // Scripts.evalJSON / saveJSON
        {TJS_N("layerExDraw.dll"), true},  // GdiPlus + Layer.clear/drawString/drawImage
        {TJS_N("sqlite3.dll"), true},      // Sqlite class and the XP3-backed VFS
        {TJS_N("emoteplayer.dll"), true},  // Motion / EmotePlayer (E-mote)
        {TJS_N("psd.dll"), true},          // PSD class and the psd:// storage media
        {TJS_N("wuffmpeg.dll"), true},     // decoder in host (alias)
        {TJS_N("wuopus.dll"), true},       // decoder in host (alias)
        {TJS_N("wuvorbis.dll"), true},     // decoder in host (alias)
    };

    for (const auto& module : modules)
    {
        // A missing module only shows up as a script error later ("member not
        // found"), so the failure is recorded here where the cause is obvious.
        if (!TVPLoadInternalPlugin(module.name) && module.built)
            TVPAddLog(ttstr(TJS_N("(warn) internal plugin not registered: ")) + module.name);
    }
}

void TVPUnloadInternalPlugins()
{
    ncbAutoRegister::ClearRegisteredModule();
}

bool TVPLoadInternalPlugin(const ttstr& _name)
{
    // Retail KiriKiri ships plugins as .dll or as .tpm (a renamed PE image),
    // and games write both spellings: `Plugins.link("layerExDraw.dll")` and
    // `Plugins.link("layerExDraw.tpm")`. The module registry is keyed by the
    // compiled-in NCB module name ("layerExDraw.dll"), so normalize the
    // basename's extension before the lookup instead of failing the .tpm form.
    ttstr name = TVPExtractStorageName(_name);
    if (name.length() > 4)
    {
        const tjs_char* tail = name.c_str() + name.length() - 4;
        if (TJS_strcasecmp(tail, TJS_N(".tpm")) == 0)
            name = ttstr(name.c_str(), static_cast<tjs_int>(name.length() - 4)) + TJS_N(".dll");
    }
    if (ncbAutoRegister::LoadModule(name))
        return true;
    return TVPLoadAliasedPlugin(name);
}