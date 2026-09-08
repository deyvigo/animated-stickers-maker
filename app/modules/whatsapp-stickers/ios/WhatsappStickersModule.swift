import ExpoModulesCore

/// iOS is out of scope for the MVP (see the project plan) — WhatsApp's iOS
/// sticker integration uses a different mechanism entirely (UIPasteboard
/// with the `net.whatsapp.WAStickers` type), not this Android
/// ContentProvider contract. These stubs keep the app buildable on iOS
/// dev-client builds without crashing; implement for real if iOS support
/// gets picked up later.
public class WhatsappStickersModule: Module {
  public func definition() -> ModuleDefinition {
    Name("WhatsappStickers")

    AsyncFunction("isWhatsAppInstalled") { () -> [String: Bool] in
      return ["consumer": false, "business": false]
    }

    AsyncFunction("isPackAdded") { (_ packId: String) -> Bool in
      return false
    }

    AsyncFunction("addPackToWhatsApp") { (_ packId: String) -> String in
      throw NotSupportedOnPlatformException()
    }
  }
}

internal struct NotSupportedOnPlatformException: Error, CustomStringConvertible {
  var description: String {
    "Agregar a WhatsApp todavía no está implementado en iOS."
  }
}
