#import <Cocoa/Cocoa.h>
#include "about_darwin.h"

static NSAttributedString *credits(NSString *credit, NSString *link) {
  NSMutableParagraphStyle *centred = [[NSMutableParagraphStyle alloc] init];
  centred.alignment = NSTextAlignmentCenter;
  NSDictionary *plain = @{
    NSFontAttributeName : [NSFont systemFontOfSize:11],
    NSForegroundColorAttributeName : [NSColor labelColor],
    NSParagraphStyleAttributeName : centred,
  };
  NSMutableAttributedString *text =
      [[NSMutableAttributedString alloc] initWithString:[credit stringByAppendingString:@"\n"] attributes:plain];
  NSMutableDictionary *linked = [plain mutableCopy];
  linked[NSLinkAttributeName] = [NSURL URLWithString:link];
  [text appendAttributedString:[[NSAttributedString alloc] initWithString:link attributes:linked]];
  return text;
}

void LandlineUseAccessoryPolicy(void) {
  dispatch_async(dispatch_get_main_queue(), ^{
    [NSApplication sharedApplication];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
  });
}

void LandlineShowAbout(const char *name, const char *version, const void *icon, int iconLength, const char *credit, const char *link) {
  NSString *appName = [NSString stringWithUTF8String:name];
  NSString *appVersion = [NSString stringWithUTF8String:version];
  NSData *iconData = [NSData dataWithBytes:icon length:iconLength];
  NSString *creditText = [NSString stringWithUTF8String:credit];
  NSString *linkText = [NSString stringWithUTF8String:link];
  dispatch_async(dispatch_get_main_queue(), ^{
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    [NSApp activateIgnoringOtherApps:YES];
    [NSApp orderFrontStandardAboutPanelWithOptions:@{
      @"ApplicationName" : appName,
      @"ApplicationIcon" : [[NSImage alloc] initWithData:iconData],
      @"ApplicationVersion" : appVersion,
      @"Version" : @"",
      @"Credits" : credits(creditText, linkText),
    }];
    __block id observer = [[NSNotificationCenter defaultCenter]
        addObserverForName:NSWindowWillCloseNotification
                    object:nil
                     queue:[NSOperationQueue mainQueue]
                usingBlock:^(NSNotification *note) {
                  [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
                  [[NSNotificationCenter defaultCenter] removeObserver:observer];
                }];
  });
}
