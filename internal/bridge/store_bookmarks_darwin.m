//go:build appstore && darwin

#import <Foundation/Foundation.h>
#import <dispatch/dispatch.h>

int modelbridge_acquire_folder(const char *home, const char *client) {
    @autoreleasepool {
        NSString *key = [NSString stringWithUTF8String:client];
        if (![@[@"codex", @"claude", @"copilot"] containsObject:key]) return 0;
        NSString *root = [NSString stringWithUTF8String:home];
        NSString *file = [[root stringByAppendingPathComponent:@".kobashi"] stringByAppendingPathComponent:@"client-access.plist"];
        NSDictionary *bookmarks = [NSDictionary dictionaryWithContentsOfFile:file];
        NSData *bookmark = bookmarks[key];
        if (![bookmark isKindOfClass:[NSData class]]) return 0;
        BOOL stale = NO;
        NSError *error = nil;
        NSURL *url = [NSURL URLByResolvingBookmarkData:bookmark options:NSURLBookmarkResolutionWithSecurityScope | NSURLBookmarkResolutionWithoutUI relativeToURL:nil bookmarkDataIsStale:&stale error:&error];
        if (!url || error || stale) return 0;
        BOOL scoped = [url startAccessingSecurityScopedResource];
        NSString *resolvedRoot = root.stringByStandardizingPath.stringByResolvingSymlinksInPath;
        NSString *resolvedPath = url.path.stringByStandardizingPath.stringByResolvingSymlinksInPath;
        if (!scoped && ![resolvedPath hasPrefix:[resolvedRoot stringByAppendingString:@"/"]]) return 0;
        NSString *link = [root stringByAppendingPathComponent:[@"." stringByAppendingString:key]];
        NSFileManager *manager = NSFileManager.defaultManager;
        NSString *existing = [manager destinationOfSymbolicLinkAtPath:link error:nil];
        if (existing && ![existing isEqualToString:url.path]) {
            [url stopAccessingSecurityScopedResource];
            return 0; // Never replace a client's saved mapping while it may be active.
        }
        if (!existing && ![manager createSymbolicLinkAtPath:link withDestinationPath:url.path error:&error]) {
            [url stopAccessingSecurityScopedResource];
            return 0;
        }
        // Keep scopes alive for this helper's lifetime; termination revokes them.
        static NSMutableDictionary *active;
        static NSObject *lock;
        static dispatch_once_t once;
        dispatch_once(&once, ^{ active = [NSMutableDictionary new]; lock = [NSObject new]; });
        @synchronized(lock) {
            if (active[key]) [url stopAccessingSecurityScopedResource];
            else active[key] = url;
        }
        return 1;
    }
}
