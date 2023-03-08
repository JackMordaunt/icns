#include <objbase.h>
#include <thumbcache.h>
#include <shobjidl.h>
#include <windows.h>
#include <INITGUID.h>


// OutstandingObjects counts references to objects created. 
// When zero, it is safe to unload the DLL.
static volatile long OutstandingObjects = 0;

// LockCount counts invocations to classLockServer.
static volatile long LockCount = 0;

// {CA7786E8-C694-49AF-AFC1-DB80EDBCEDAE}
DEFINE_GUID(CLSID_ICNSThumbnailProvider, 0xca7786e8, 0xc694, 0x49af, 0xaf, 0xc1, 0xdb, 0x80, 0xed, 0xbc, 0xed, 0xae);

// {2D03DA50-13A7-4166-AD3D-E3EAE6244C1C}
DEFINE_GUID(IID_ICNSThumbnailProvider, 0x2d03da50, 0x13a7, 0x4166, 0xad, 0x3d, 0xe3, 0xea, 0xe6, 0x24, 0x4c, 0x1c);

// Define the object. 
typedef struct ICNSThumbnailProvider {
    IThumbnailProviderVtbl * lpVtbl;
    DWORD refCount; 
} ICNSThumbnailProvider;

HRESULT
STDMETHODCALLTYPE
QueryInterface(
    IThumbnailProvider     *this,
    REFIID                vTableGuid,
    void                  **ppv    
) {
    // If the caller is not asking for an interface we support, return an error.
    if (!IsEqualIID(vTableGuid, &IID_ICNSThumbnailProvider) && !IsEqualIID(vTableGuid, &IID_IUnknown)) {
        *ppv = 0;
        return E_NOINTERFACE;
    }

    // Fill in the pointer with our object.
    *ppv = this;

    // Incrememt the reference counter. 
    this->lpVtbl->AddRef(this);

    return NOERROR;
}

ULONG
STDMETHODCALLTYPE
AddRef(IThumbnailProvider *this) {
    ICNSThumbnailProvider *self = (ICNSThumbnailProvider*)this;
    self->refCount += 1;
    return self->refCount;
}

ULONG
STDMETHODCALLTYPE
Release(IThumbnailProvider *this) {
    ICNSThumbnailProvider *self = (ICNSThumbnailProvider*)this;
    self->refCount -= 1;
    
    if (self->refCount == 0) {
        GlobalFree(self);
        InterlockedDecrement(&OutstandingObjects);
        return 0;
    }
    
    return self->refCount;
}

HRESULT
STDMETHODCALLTYPE
GetThumbnail(
    // [in] this
    IThumbnailProvider *this,
    
    // [in] cx
    // The maximum thumbnail size, in pixels. The Shell draws the returned bitmap at
    // this size or smaller. The returned bitmap should fit into a square of width
    // and height cx, though it does not need to be a square image. The Shell scales
    // the bitmap to render at lower sizes. For example, if the image has a 6:4 aspect
    // ratio, then the returned bitmap should also have a 6:4 aspect ratio.   
    UINT cx,                

    // [out] phbmp
    // When this method returns, contains a pointer to the thumbnail image handle.
    // The image must be a DIB section and 32 bits per pixel. The Shell scales down
    // the bitmap if its width or height is larger than the size specified by cx. The
    // Shell always respects the aspect ratio and never scales a bitmap larger than its
    // original size.   
    HBITMAP *phbmp,         
    
    // [out] pdwAlpha
    // When this method returns, contains a pointer to one of the following values from
    // the WTS_ALPHATYPE enumeration:
    // - WTSAT_UNKNOWN (0x0)
    //     0x0. The bitmap is an unknown format. The Shell tries nonetheless to detect
    //     whether the image has an alpha channel.
    // - WTSAT_RGB (0x1)
    //     0x1. The bitmap is an RGB image without alpha. The alpha channel is invalid
    //     and the Shell ignores it.
    // - WTSAT_ARGB (0x2)
    //     0x2. The bitmap is an ARGB image with a valid alpha channel.    
    WTS_ALPHATYPE *pdwAlpha 
) {
    
    return NOERROR;
}

static IThumbnailProviderVtbl ICNSThumbnailProvider_Vtbl = {
    QueryInterface,
    AddRef,
    Release,
    GetThumbnail
}; 

// classQueryInterface implements the COM class query, returning NOERROR if the
// provided object is in fact an IClassFactory. 
HRESULT
STDMETHODCALLTYPE
classQueryInterface(
    IClassFactory *this,
    REFIID factoryGuid, 
    void **ppv
) {
    if (!IsEqualIID(factoryGuid, &IID_IUnknown) && !IsEqualIID(factoryGuid, &IID_IClassFactory)) {
        *ppv = 0;
        return E_NOINTERFACE;
    }

    *ppv = this;

    this->lpVtbl->AddRef(this);
    
    return NOERROR;
}

// classAddRef always returns 1 beacuse the class is statically allocated. 
ULONG
STDMETHODCALLTYPE
classAddRef(IClassFactory *this) {
    return 1;
}

// classRelease always returns 1 beacuse the class is statically allocated. 
ULONG
STDMETHODCALLTYPE
classRelease(IClassFactory *this) {
    return 1;
}

// classLockServer allows the caller to manually lock the DLL so that it
// doesn't get unloaded even when no objects are currently allocated. 
HRESULT
STDMETHODCALLTYPE
classLockServer(IClassFactory *this, BOOL flock) {
    if (flock) {
        InterlockedIncrement(&LockCount);
    } else {
        InterlockedDecrement(&LockCount);
    }
    return NOERROR;
}

HRESULT
STDMETHODCALLTYPE
classCreateInstance(
    IClassFactory *this,
    IUnknown *aggregate,
    REFIID vTableGuid,
    void **ppv    
) {
    HRESULT hr;
    ICNSThumbnailProvider *self;

    *ppv = 0;

    if (aggregate != NULL) {
        return CLASS_E_NOAGGREGATION;
    }

    self = GlobalAlloc(GMEM_FIXED, sizeof(ICNSThumbnailProvider));

    if (self == NULL) {
        return E_OUTOFMEMORY;
    }

    self->lpVtbl = &ICNSThumbnailProvider_Vtbl;

    self->refCount = 1;

    hr =  ICNSThumbnailProvider_Vtbl.QueryInterface((IThumbnailProvider*)self, vTableGuid, ppv);

    ICNSThumbnailProvider_Vtbl.Release((IThumbnailProvider*)self);

    if (hr == NOERROR) {
        InterlockedIncrement(&OutstandingObjects);
    }
    
    return hr;
}

// IClassFactoryVtbl is a statically defined vtable for our class factory. 
static IClassFactoryVtbl IClassFactory_Vtbl = {
    classQueryInterface,
    classAddRef,
    classRelease,
    classCreateInstance,
    classLockServer
};

// ICNSThumbnailProviderFactory is the COM objec that instances 
// ICNSThumbnailProvider objects for use. 
static IClassFactory ICNSThumbnailProviderFactory = {&IClassFactory_Vtbl};

HRESULT
PASCAL
DllGetClassObject(
    REFCLSID objGuid, 
    REFIID factoryGuid,
    void **factoryHandle
) {
   HRESULT  hr;

   if (IsEqualCLSID(objGuid, &CLSID_ICNSThumbnailProvider))
   {
        // Fill in the caller's handle with a pointer to our Factory object.
        return classQueryInterface(&ICNSThumbnailProviderFactory, factoryGuid, factoryHandle);
   }
    // We don't understand this GUID.
    // It's obviously not for our DLL.
    // Let the caller know this by
    // clearing his handle and returning
    // CLASS_E_CLASSNOTAVAILABLE.
    *factoryHandle = 0;
    return CLASS_E_CLASSNOTAVAILABLE;
}

