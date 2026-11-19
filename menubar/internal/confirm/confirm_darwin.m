#import <Cocoa/Cocoa.h>
#include "confirm_darwin.h"

int LandlineConfirm(const char *question, const char *accept, const char *decline) {
  NSString *questionText = [NSString stringWithUTF8String:question];
  NSString *acceptText = [NSString stringWithUTF8String:accept];
  NSString *declineText = [NSString stringWithUTF8String:decline];
  __block BOOL accepted = NO;
  void (^ask)(void) = ^{
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = questionText;
    [alert addButtonWithTitle:declineText];
    [alert addButtonWithTitle:acceptText];
    [NSApp activateIgnoringOtherApps:YES];
    accepted = [alert runModal] == NSAlertSecondButtonReturn;
  };
  if ([NSThread isMainThread]) {
    ask();
  } else {
    dispatch_sync(dispatch_get_main_queue(), ask);
  }
  return accepted ? 1 : 0;
}
