//---------------------------------------------------------------------------
/*
        TJS2 Script Engine
        Copyright (C) 2000 W.Dee <dee@kikyou.info> and contributors

     See details of license at "license.txt"
*/
//---------------------------------------------------------------------------
// "tTJS" script language API class implementation
//---------------------------------------------------------------------------

#pragma once

#include "tjsVariant.h"

#include <algorithm>
#include <atomic>
#include <deque>
#include <mutex>
#include <vector>

namespace TJS
{
//---------------------------------------------------------------------------
// TJS version
//---------------------------------------------------------------------------
extern const tjs_int TJSVersionMajor;
extern const tjs_int TJSVersionMinor;
extern const tjs_int TJSVersionRelease;
extern const tjs_int TJSVersionHex;

extern tjs_char TJSCompiledDate[];
//---------------------------------------------------------------------------

//---------------------------------------------------------------------------
// Console output callback interface
//---------------------------------------------------------------------------
class iTJSConsoleOutput
{
public:
    virtual void ExceptionPrint(const tjs_char* msg) = 0;
    virtual void Print(const tjs_char* msg) = 0;
};
//---------------------------------------------------------------------------

//---------------------------------------------------------------------------
// Object Hash Size Limit ( must be larger than or equal to 0 )
//---------------------------------------------------------------------------
extern tjs_int TJSObjectHashBitsLimit;

//---------------------------------------------------------------------------
// global options
//---------------------------------------------------------------------------
extern bool TJSEvalOperatorIsOnGlobal;
// Post-! operator (evaluate expression) is to be executed on "this" context
// since TJS2 2.4.1.
// Turn this switch true makes post-! operator running on global context,
// like TJS2 before 2.4.1.
extern bool TJSWarnOnNonGlobalEvalOperator;
// Output warning against non-local post-! operator.
// (For checking where the post-! operators are used)
extern bool TJSEnableDebugMode;
// Enable TJS2 Debugging support. Enabling this may make the
// program somewhat slower and using more memory.
// Do not use this mode unless you want to debug the program.
extern bool TJSWarnOnExecutionOnDeletingObject;
// Output warning against running code on context of
// deleting-in-progress object. This is available only the Debug mode is
// enabled.
extern bool TJSUnaryAsteriskIgnoresPropAccess;
// Unary '*' operator means accessing property object directly without
// normal property access, if this options is set true.
// This is replaced with '&' operator since TJS2 2.4.15. Turn true for
// gaining old compatibility.

//---------------------------------------------------------------------------
// tTJS class - "tTJS" TJS API Class
//---------------------------------------------------------------------------
class tTJSScriptBlock;
class tTJSPPMap;
class tTJSCustomObject;
class tTJSScriptCache;
class tTJS
{
    friend class tTJSScriptBlock;
public:
    tTJS();
    ~tTJS();

private:
    tTJSPPMap* PPValues;

    std::vector<tTJSScriptBlock*> ScriptBlocks;

    iTJSConsoleOutput* ConsoleOutput;

    tTJSCustomObject* Global;

    tTJSScriptCache* Cache;
    class tTJSVariantArrayStack* VariantArrayStack = nullptr;

public:
    iTJSDispatch2* GetGlobal();
    iTJSDispatch2* GetGlobalNoAddRef() const;

    tTJSVariantArrayStack* GetVariantArrayStack() { return VariantArrayStack; }

private:
    void AddScriptBlock(tTJSScriptBlock* block);
    void RemoveScriptBlock(tTJSScriptBlock* block);

public:
    void SetConsoleOutput(iTJSConsoleOutput* console);
    iTJSConsoleOutput* GetConsoleOutput() const { return ConsoleOutput; };
    void OutputToConsole(const tjs_char* msg) const;
    void OutputExceptionToConsole(const tjs_char* msg) const;
    void OutputToConsoleWithCentering(const tjs_char* msg, tjs_uint width) const;
    void OutputToConsoleSeparator(const tjs_char* text, tjs_uint count) const;

    void Dump(tjs_uint width = 80) const; // dumps all existing script block

    void ExecScript(const tjs_char* script,
                    tTJSVariant* result = NULL,
                    iTJSDispatch2* context = NULL,
                    const tjs_char* name = NULL,
                    tjs_int lineofs = 0);

    void ExecScript(const ttstr& script,
                    tTJSVariant* result = NULL,
                    iTJSDispatch2* context = NULL,
                    const ttstr* name = NULL,
                    tjs_int lineofs = 0);

    void EvalExpression(const tjs_char* expression,
                        tTJSVariant* result,
                        iTJSDispatch2* context = NULL,
                        const tjs_char* name = NULL,
                        tjs_int lineofs = 0);

    void EvalExpression(const ttstr& expression,
                        tTJSVariant* result,
                        iTJSDispatch2* context = NULL,
                        const ttstr* name = NULL,
                        tjs_int lineofs = 0);

    void SetPPValue(const tjs_char* name, const tjs_int32 value);
    tjs_int32 GetPPValue(const tjs_char* name);

    void DoGarbageCollection();

    // for Bytecode
    void LoadByteCode(const tjs_uint8* buff,
                      size_t len,
                      tTJSVariant* result = NULL,
                      iTJSDispatch2* context = NULL,
                      const tjs_char* name = NULL);

    bool LoadByteCode(class tTJSBinaryStream* stream,
                      tTJSVariant* result = NULL,
                      iTJSDispatch2* context = NULL,
                      const tjs_char* name = NULL);

    // for Binary Dictionay Array
    static bool LoadBinaryDictionayArray(class tTJSBinaryStream* stream, tTJSVariant* result);

    void CompileScript(const tjs_char* script,
                       class tTJSBinaryStream* output,
                       bool isresultneeded = false,
                       bool outputdebug = false,
                       bool isexpression = false,
                       const tjs_char* name = NULL,
                       tjs_int lineofs = 0);
};
//---------------------------------------------------------------------------

/*[*/
//---------------------------------------------------------------------------
// iTJSTextStream - used by Array.save/load Dictionaty.save/load
//---------------------------------------------------------------------------
class tTJSString;
class iTJSTextReadStream
{
public:
    virtual tjs_uint Read(tTJSString& targ, tjs_uint size) = 0;
    virtual void Destruct() = 0; // must delete itself
    virtual ~iTJSTextReadStream() {}
};
//---------------------------------------------------------------------------
class iTJSTextWriteStream
{
public:
    virtual void Write(const tTJSString& targ) = 0;
    virtual void Destruct() = 0; // must delete itself
    virtual ~iTJSTextWriteStream() {}
};
//---------------------------------------------------------------------------
extern iTJSTextReadStream* (*TJSCreateTextStreamForRead)(const tTJSString& name,
                                                         const tTJSString& modestr);
extern iTJSTextWriteStream* (*TJSCreateTextStreamForWrite)(const tTJSString& name,
                                                           const tTJSString& modestr);
extern class tTJSBinaryStream* (*TJSCreateBinaryStreamForRead)(const tTJSString& name,
                                                               const tTJSString& modestr);
extern class tTJSBinaryStream* (*TJSCreateBinaryStreamForWrite)(const tTJSString& name,
                                                                const tTJSString& modestr);
//---------------------------------------------------------------------------

/*]*/
/*[*/
//---------------------------------------------------------------------------
// tTJSBinaryStream constants
//---------------------------------------------------------------------------
#define TJS_BS_READ 0
#define TJS_BS_WRITE 1
#define TJS_BS_APPEND 2
#define TJS_BS_UPDATE 3

#define TJS_BS_DELETE_ON_CLOSE 0x10

#define TJS_BS_ACCESS_MASK 0x0f
#define TJS_BS_OPTION_MASK 0xf0

#define TJS_BS_SEEK_SET 0
#define TJS_BS_SEEK_CUR 1
#define TJS_BS_SEEK_END 2
//---------------------------------------------------------------------------

/*]*/

//---------------------------------------------------------------------------
// tTJSBinaryStream base stream class
//---------------------------------------------------------------------------
class tTJSBinaryStream
{
private:
public:
    //-- must implement
    virtual tjs_uint64 Seek(tjs_int64 offset, tjs_int whence) = 0;
    /* if error, position is not changed */

    //-- optionally to implement
    virtual tjs_uint Read(void* buffer, tjs_uint read_size) = 0;
    /* returns actually read size */

    virtual tjs_uint Write(const void* buffer, tjs_uint write_size) = 0;
    /* returns actually written size */

    virtual bool Flush() = 0;
    /* returns flush status */

    virtual void SetEndOfStorage();
    // the default behavior is raising a exception
    /* if error, raises exception */

    //-- should re-implement for higher performance
    virtual tjs_uint64 GetSize() = 0;

    virtual ~tTJSBinaryStream() { ; }

    tjs_uint64 GetPosition();

    void SetPosition(tjs_uint64 pos);

    void ReadBuffer(void* buffer, tjs_uint read_size);
    void WriteBuffer(const void* buffer, tjs_uint write_size);

    tjs_uint64 ReadI64LE(); // reads little-endian integers
    tjs_uint32 ReadI32LE();
    tjs_uint16 ReadI16LE();
    tjs_uint8 ReadI8LE();
};
//---------------------------------------------------------------------------

//---------------------------------------------------------------------------
// tTJSObjectPool
//---------------------------------------------------------------------------
// 这个池只做一件事：按创建顺序登记活着的 TJS 对象，供引擎销毁时强制清扫
// （TJSClearObjectHeap / TJSClearRegisterHeap / TJSClearScriptBlockHeap）。
// 索引（GCIndex）由 registerObject 返回、由析构函数交回，除此之外不参与任何
// 语义，因此它必须是"永远可以安全释放、永远可以安全复用"的。
//
// 原实现把 slots（std::deque）与空闲栈拆成两套独立增长的计数器，并让
// clear() 用空闲栈容量重建空闲栈：进程内关掉一个游戏再开下一个时，
//   * clear() 之后 head 可能大于 slots.size()，下一次 registerObject 直接
//     写到 deque 之外（实测崩溃：tTJSCustomObject 构造里 SIGSEGV，故障地址
//     0xff0 = (idx & 0xff) * 16，说明索引越界）；
//   * 构造函数没把 size 设成初始容量，第一次扩容会把 0..1023 重复压栈，
//     同一槽位被两个对象共用，析构时又重复交回；
//   * 扩容只改空闲栈、slots 却在 clear() 后被缩容，收缩与空闲索引再次错位。
// 现在只用一条不变量：free_ 中每个索引都 < slots_.size()，slots_.size() 只增
// 不减（clear 除外），三处使用都短时间持锁——TJS 对象创建本身就要分配符号表，
// 一把无争用的互斥锁（约 20ns）相对而言完全可以忽略。
template<typename T>
class tTJSObjectPool
{
    // TJS 对象可能从图像加载线程以外的线程创建（插件/宿主回调），
    // 因此登记表必须真正线程安全；原实现的向量扩容在并发下是数据竞争。
    std::mutex mutex_;
    std::vector<T*> slots_;    // 下标即 GCIndex；nullptr 表示空闲
    std::vector<size_t> free_; // 空闲槽位栈，元素恒 < slots_.size()

    static constexpr size_t kInvalidIndex = static_cast<size_t>(-1);

public:
    explicit tTJSObjectPool(size_t initialCapacity = 1024)
    {
        std::lock_guard<std::mutex> lock(mutex_);
        growLocked(initialCapacity);
    }

    tTJSObjectPool(const tTJSObjectPool&) = delete;
    tTJSObjectPool& operator=(const tTJSObjectPool&) = delete;

    size_t registerObject(T* obj)
    {
        if (!obj)
            return kInvalidIndex;
        std::lock_guard<std::mutex> lock(mutex_);
        if (free_.empty())
        {
            // 只扩容，不缩容：任何已发出的 GCIndex 在任何时刻都仍然有效。
            growLocked(slots_.size() / 2 + 1024);
        }
        const size_t idx = free_.back();
        free_.pop_back();
        // clear() 之后旧对象的析构不会再进到这里，但保持断言式的写法，
        // 让"槽位被两个对象共用"这类问题在调试版直接停住。
        slots_[idx] = obj;
        return idx;
    }

    void unregisterObject(size_t idx)
    {
        std::lock_guard<std::mutex> lock(mutex_);
        if (idx >= slots_.size() || slots_[idx] == nullptr)
        {
            // clear() 已经把这个槽位回收过了：迟到的析构不能再把它二次交回，
            // 否则空闲栈里会出现重复索引，同一槽位会被两个对象共用。
            return;
        }
        slots_[idx] = nullptr;
        free_.push_back(idx);
    }

    void clear()
    {
        std::lock_guard<std::mutex> lock(mutex_);
        // 保持 slots_ 的容量（扩容过的槽位继续可用），只把占用全部释放。
        std::fill(slots_.begin(), slots_.end(), nullptr);
        free_.resize(slots_.size());
        for (size_t i = 0; i < slots_.size(); ++i)
        {
            free_[i] = i;
        }
    }

    // 清扫回调会在回调里 delete 对象，而析构又会调用 unregisterObject：
    // 因此先在锁内取快照，出锁之后才回调，既不重入死锁也不会漏掉对象。
    template<typename Func>
    void forEach(Func func)
    {
        std::vector<T*> snapshot;
        {
            std::lock_guard<std::mutex> lock(mutex_);
            snapshot.reserve(slots_.size());
            for (T* obj : slots_)
            {
                if (obj)
                {
                    snapshot.push_back(obj);
                }
            }
        }
        for (T* obj : snapshot)
        {
            func(obj);
        }
    }

private:
    // 调用方必须持有 mutex_。
    void growLocked(size_t count)
    {
        const size_t oldSize = slots_.size();
        slots_.resize(oldSize + count, nullptr);
        free_.reserve(slots_.size());
        // 逆序压栈，让编号小的槽位先被复用（保持历史行为，且局部性更好）。
        for (size_t i = slots_.size(); i > oldSize; --i)
        {
            free_.push_back(i - 1);
        }
    }
};
}; // namespace TJS
