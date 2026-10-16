import { QueueScreen } from '@/screens/QueueScreen';
import { useSession } from '@/session';

export default function QueueRoute() {
  const session = useSession();
  if (session.status !== 'ready') return null;
  return <QueueScreen client={session.client} warning={session.warning} />;
}
